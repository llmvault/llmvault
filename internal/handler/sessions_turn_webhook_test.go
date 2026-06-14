package handler_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/hibiken/asynq"

	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/tasks"
)

type recordingEnqueuer struct {
	tasks []*asynq.Task
}

func (r *recordingEnqueuer) Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	return r.EnqueueContext(context.Background(), task, opts...)
}

func (r *recordingEnqueuer) EnqueueContext(_ context.Context, task *asynq.Task, _ ...asynq.Option) (*asynq.TaskInfo, error) {
	r.tasks = append(r.tasks, task)
	return &asynq.TaskInfo{}, nil
}

func (r *recordingEnqueuer) Close() error { return nil }

func TestIntegration_RuntimeTurnCompletedWebhookDrainsQueuedSessionMessage(t *testing.T) {
	h := newSessionHarness(t)
	fx := h.seed(t)
	created := h.createSession(t, fx, fx.owner, "First turn")
	second := h.doJSON(t, http.MethodPost, "/v1/sessions/"+created.Session.ID+"/messages", fx, fx.owner, map[string]any{
		"text": "Second turn",
	})
	if second.Code != http.StatusAccepted {
		t.Fatalf("second message status=%d body=%s", second.Code, second.Body.String())
	}

	runtimeSecret := "runtime-webhook-drain-secret"
	encSecret, err := sessionTestEncKey(t).EncryptString(runtimeSecret)
	if err != nil {
		t.Fatalf("encrypt runtime secret: %v", err)
	}
	sb := model.Sandbox{
		OrgID:                  &fx.org.ID,
		AgentID:                &fx.agent.ID,
		ProviderID:             "docker",
		ExternalID:             "webhook-drain-container",
		RuntimeURL:             "http://203.0.113.10:7080",
		EncryptedRuntimeSecret: encSecret,
		Status:                 "running",
	}
	if err := h.db.Create(&sb).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	startedAt := time.Now().Add(-time.Minute)
	if err := h.db.Model(&model.Session{}).Where("id = ?", created.Session.ID).Updates(map[string]any{
		"sandbox_id":            sb.ID,
		"agent_turn_status":     model.SessionAgentTurnActive,
		"agent_turn_id":         "turn-1",
		"agent_stream_id":       "stream-1",
		"agent_turn_started_at": startedAt,
	}).Error; err != nil {
		t.Fatalf("mark session active: %v", err)
	}
	if err := h.db.Model(&model.SessionMessageQueue{}).Where("session_event_id = ?", created.Event.ID).Updates(map[string]any{
		"status":             "delivered",
		"delivered_at":       time.Now(),
		"runtime_stream_id":  "stream-1",
		"runtime_stream_url": "/sessions/" + created.Session.ID + "/stream",
		"runtime_turn_id":    "turn-1",
	}).Error; err != nil {
		t.Fatalf("mark first queue delivered: %v", err)
	}

	enq := &recordingEnqueuer{}
	webhooks := handler.NewAgentOutboundWebhookHandler(h.db, sessionTestEncKey(t), enq)
	body := signedRuntimeEventBody(t, runtimeSecret, map[string]any{
		"event_type": "turn_completed",
		"payload": map[string]any{
			"session_id": created.Session.ID,
			"turn_id":    "turn-1",
			"event_id":   "turn-completed-1",
		},
		"at": time.Now().UTC().Format(time.RFC3339Nano),
	})
	req := httptest.NewRequest(http.MethodPost, "/internal/webhooks/agent/"+sb.ID.String(), bytes.NewReader(body.raw))
	req.Header.Set("X-Hivy-Signature", body.signature)
	rr := httptest.NewRecorder()
	r := chi.NewRouter()
	r.Post("/internal/webhooks/agent/{sandboxID}", webhooks.Handle)
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("webhook status=%d body=%s", rr.Code, rr.Body.String())
	}

	var session model.Session
	if err := h.db.First(&session, "id = ?", created.Session.ID).Error; err != nil {
		t.Fatalf("reload session: %v", err)
	}
	if session.AgentTurnStatus != model.SessionAgentTurnIdle || session.AgentTurnID != "" || session.AgentStreamID != "" || session.AgentTurnStartedAt != nil {
		t.Fatalf("session turn was not released: %+v", session)
	}
	if !hasTaskType(enq.tasks, tasks.TypeSessionMessageDeliver) {
		t.Fatalf("queued tasks=%v", taskTypes(enq.tasks))
	}
}

type signedRuntimeEvent struct {
	raw       []byte
	signature string
}

func signedRuntimeEventBody(t *testing.T, secret string, payload map[string]any) signedRuntimeEvent {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal runtime event: %v", err)
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(raw)
	return signedRuntimeEvent{raw: raw, signature: "sha256=" + hex.EncodeToString(mac.Sum(nil))}
}

func taskTypes(tasks []*asynq.Task) []string {
	out := make([]string, len(tasks))
	for i, task := range tasks {
		out[i] = task.Type()
	}
	return out
}

func hasTaskType(tasks []*asynq.Task, taskType string) bool {
	for _, task := range tasks {
		if task.Type() == taskType {
			return true
		}
	}
	return false
}
