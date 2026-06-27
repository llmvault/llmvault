package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/usehivy/hivy/internal/model"
	sandboxpkg "github.com/usehivy/hivy/internal/sandbox"
)

func TestIntegration_SessionsInterrupt_ProxiesToRuntimeAndReleasesTurn(t *testing.T) {
	h := newSessionHarness(t)
	fx := h.seed(t)
	created := h.createSession(t, fx, fx.owner, "Stop this turn")

	runtimeSecret := "runtime-interrupt-secret"
	var runtimeCalls int
	var gotAuthorization string
	runtime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		runtimeCalls++
		gotAuthorization = r.Header.Get("Authorization")
		if r.Method != http.MethodPost || r.URL.Path != "/sessions/"+created.Session.ID+"/interrupt" {
			t.Fatalf("unexpected runtime request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"interrupted": true,
			"status":      "interrupted",
		})
	}))
	t.Cleanup(runtime.Close)

	encSecret, err := sessionTestEncKey(t).EncryptString(runtimeSecret)
	if err != nil {
		t.Fatalf("encrypt runtime secret: %v", err)
	}
	sb := model.Sandbox{
		OrgID:                  &fx.org.ID,
		AgentID:                &fx.agent.ID,
		ProviderID:             sandboxpkg.ProviderMicrosandbox,
		ExternalID:             "interrupt-runtime",
		RuntimeURL:             runtime.URL,
		EncryptedRuntimeSecret: encSecret,
		Status:                 "running",
	}
	if err := h.db.Create(&sb).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	startedAt := time.Now().UTC()
	if err := h.db.Model(&model.Session{}).Where("id = ?", created.Session.ID).Updates(map[string]any{
		"sandbox_id":            sb.ID,
		"agent_turn_status":     model.SessionAgentTurnActive,
		"agent_turn_id":         "turn-123",
		"agent_stream_id":       "stream-123",
		"agent_turn_started_at": startedAt,
	}).Error; err != nil {
		t.Fatalf("mark session active: %v", err)
	}

	path := "/v1/sessions/" + created.Session.ID + "/interrupt"
	blocked := h.doJSON(t, http.MethodPost, path, fx, fx.member, nil)
	if blocked.Code != http.StatusForbidden {
		t.Fatalf("unshared member interrupt status=%d body=%s", blocked.Code, blocked.Body.String())
	}

	rr := h.doJSON(t, http.MethodPost, path, fx, fx.owner, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("interrupt status=%d body=%s", rr.Code, rr.Body.String())
	}
	var out struct {
		SessionID   string `json:"session_id"`
		Interrupted bool   `json:"interrupted"`
		Status      string `json:"status"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode interrupt response: %v\n%s", err, rr.Body.String())
	}
	if out.SessionID != created.Session.ID || !out.Interrupted || out.Status != "interrupted" {
		t.Fatalf("bad interrupt response: %+v", out)
	}
	if runtimeCalls != 1 {
		t.Fatalf("runtime calls=%d, want 1", runtimeCalls)
	}
	if gotAuthorization != "Bearer "+runtimeSecret {
		t.Fatalf("runtime authorization=%q", gotAuthorization)
	}

	var session model.Session
	if err := h.db.First(&session, "id = ?", created.Session.ID).Error; err != nil {
		t.Fatalf("reload session: %v", err)
	}
	if session.AgentTurnStatus != model.SessionAgentTurnIdle || session.AgentTurnID != "" || session.AgentStreamID != "" || session.AgentTurnStartedAt != nil {
		t.Fatalf("session turn state not released: %+v", session)
	}
}
