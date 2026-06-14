package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/model"
)

func TestIntegration_SessionsStreamAccessRequiresParticipantAndReturnsStreamToken(t *testing.T) {
	h := newSessionHarness(t)
	fx := h.seed(t)
	created := h.createSession(t, fx, fx.owner, "Stream this turn")
	runtimeSecret := "runtime-stream-secret"
	encSecret, err := sessionTestEncKey(t).EncryptString(runtimeSecret)
	if err != nil {
		t.Fatalf("encrypt runtime secret: %v", err)
	}
	sb := model.Sandbox{
		OrgID:                  &fx.org.ID,
		AgentID:                &fx.agent.ID,
		ProviderID:             "docker",
		ExternalID:             "stream-access-container",
		RuntimeURL:             "http://203.0.113.10:7080",
		EncryptedRuntimeSecret: encSecret,
		Status:                 "running",
	}
	if err := h.db.Create(&sb).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	if err := h.db.Model(&model.Session{}).Where("id = ?", created.Session.ID).Update("sandbox_id", sb.ID).Error; err != nil {
		t.Fatalf("attach sandbox: %v", err)
	}
	if err := h.db.Model(&model.SessionMessageQueue{}).Where("session_event_id = ?", created.Event.ID).Updates(map[string]any{
		"status":             "delivered",
		"delivered_at":       time.Now(),
		"runtime_stream_id":  "stream-123",
		"runtime_stream_url": "/sessions/" + created.Session.ID + "/stream",
		"runtime_trace_id":   "trace-123",
		"runtime_turn_id":    "turn-123",
	}).Error; err != nil {
		t.Fatalf("mark queue delivered: %v", err)
	}

	path := "/v1/sessions/" + created.Session.ID + "/stream-access?event_id=" + created.Event.ID
	blocked := h.doJSON(t, http.MethodGet, path, fx, fx.member, nil)
	if blocked.Code != http.StatusForbidden {
		t.Fatalf("unshared member stream access status=%d body=%s", blocked.Code, blocked.Body.String())
	}

	invite := h.doJSON(t, http.MethodPut, "/v1/sessions/"+created.Session.ID+"/participants/"+fx.member.ID.String(), fx, fx.owner, nil)
	if invite.Code != http.StatusOK {
		t.Fatalf("invite status=%d body=%s", invite.Code, invite.Body.String())
	}
	allowed := h.doJSON(t, http.MethodGet, path, fx, fx.member, nil)
	if allowed.Code != http.StatusOK {
		t.Fatalf("stream access status=%d body=%s", allowed.Code, allowed.Body.String())
	}
	var out struct {
		SessionID      string `json:"session_id"`
		SessionEventID string `json:"session_event_id"`
		SequenceNumber int64  `json:"sequence_number"`
		StreamID       string `json:"stream_id"`
		StreamURL      string `json:"stream_url"`
		DirectURL      string `json:"direct_url"`
		StreamToken    string `json:"stream_token"`
		TraceID        string `json:"trace_id"`
		TurnID         string `json:"turn_id"`
	}
	if err := json.Unmarshal(allowed.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode stream access: %v\n%s", err, allowed.Body.String())
	}
	if out.SessionID != created.Session.ID || out.SessionEventID != created.Event.ID || out.SequenceNumber != 1 {
		t.Fatalf("bad stream ownership fields: %+v", out)
	}
	if out.StreamID != "stream-123" || out.TraceID != "trace-123" || out.TurnID != "turn-123" {
		t.Fatalf("bad runtime stream metadata: %+v", out)
	}
	if out.StreamURL != "/sessions/"+created.Session.ID+"/stream" {
		t.Fatalf("stream url=%q", out.StreamURL)
	}
	if out.DirectURL != "http://203.0.113.10:7080/sessions/"+created.Session.ID+"/stream" {
		t.Fatalf("direct url=%q", out.DirectURL)
	}
	if want := agentruntime.StreamTokenFromRuntimeSecret(runtimeSecret); out.StreamToken != want {
		t.Fatalf("stream token=%q want %q", out.StreamToken, want)
	}
}
