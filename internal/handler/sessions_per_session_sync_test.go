package handler_test

import (
	"errors"
	"net/http"
	"testing"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/tasks"
)

func TestIntegration_SessionsCreate_PerSessionCreatesSandboxAndSendsFirstMessage(t *testing.T) {
	runtime := newSessionSyncRuntime(t, http.StatusOK)
	h, provider := newSessionRuntimeHarness(t, runtime, nil)
	fx := h.seed(t)
	markSessionAgentPerSession(t, h, &fx)

	rr := h.doJSON(t, http.MethodPost, "/v1/sessions", fx, fx.owner, map[string]any{
		"channel_id": fx.channel.ID.String(),
		"text":       "Ship the synchronous session path",
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create session status=%d body=%s", rr.Code, rr.Body.String())
	}
	out := decodeSessionMutation(t, rr)
	if out.Queued {
		t.Fatalf("queued=%t, want false after synchronous initial delivery", out.Queued)
	}
	if out.Session.SandboxID == nil || *out.Session.SandboxID == "" {
		t.Fatalf("session sandbox_id missing: %+v", out.Session)
	}
	if len(provider.created) != 1 {
		t.Fatalf("provider creates=%d, want 1", len(provider.created))
	}
	if len(provider.deleted) != 0 {
		t.Fatalf("provider deleted=%v, want none", provider.deleted)
	}
	if count := countActiveSessionAgentProxyTokens(t, h, fx.org.ID, fx.agent.ID, out.Session.SandboxID); count == 0 {
		t.Fatalf("active sandbox agent proxy tokens=%d, want at least 1", count)
	}
	if runtime.messageCalls != 1 {
		t.Fatalf("runtime message calls=%d, want 1", runtime.messageCalls)
	}
	if runtime.lastSessionID != out.Session.ID || runtime.lastMessageText != "Ship the synchronous session path" {
		t.Fatalf("runtime message session=%q text=%q", runtime.lastSessionID, runtime.lastMessageText)
	}

	var queue model.SessionMessageQueue
	if err := h.db.First(&queue, "session_id = ?", out.Session.ID).Error; err != nil {
		t.Fatalf("load queue row: %v", err)
	}
	if queue.Status != "delivered" || queue.RuntimeTurnID == "" || queue.RuntimeStreamID == "" {
		t.Fatalf("queue not delivered synchronously: %+v", queue)
	}
	for _, task := range h.enqueuer.Tasks() {
		if task.TypeName == tasks.TypeSessionMessageDeliver {
			t.Fatalf("unexpected initial delivery task enqueued")
		}
	}

	access := h.doJSON(t, http.MethodPost, "/v1/sessions/"+out.Session.ID+"/sandbox-access", fx, fx.owner, nil)
	if access.Code != http.StatusOK {
		t.Fatalf("sandbox access status=%d body=%s", access.Code, access.Body.String())
	}
}

func TestIntegration_SessionsCreate_PerSessionSandboxFailureLeavesNoSessionRows(t *testing.T) {
	runtime := newSessionSyncRuntime(t, http.StatusOK)
	h, _ := newSessionRuntimeHarness(t, runtime, errors.New("provider unavailable"))
	fx := h.seed(t)
	markSessionAgentPerSession(t, h, &fx)

	rr := h.doJSON(t, http.MethodPost, "/v1/sessions", fx, fx.owner, map[string]any{
		"channel_id": fx.channel.ID.String(),
		"text":       "This should not create a session",
	})
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("create session status=%d body=%s", rr.Code, rr.Body.String())
	}
	assertNoSessionCreateRows(t, h, fx.org.ID)
	if count := countActiveSessionAgentProxyTokens(t, h, fx.org.ID, fx.agent.ID, nil); count != 0 {
		t.Fatalf("active agent proxy tokens=%d, want 0", count)
	}
	if runtime.messageCalls != 0 {
		t.Fatalf("runtime message calls=%d, want 0", runtime.messageCalls)
	}
}

func TestIntegration_SessionsCreate_PerSessionFirstMessageFailureCleansUp(t *testing.T) {
	runtime := newSessionSyncRuntime(t, http.StatusInternalServerError)
	h, provider := newSessionRuntimeHarness(t, runtime, nil)
	fx := h.seed(t)
	markSessionAgentPerSession(t, h, &fx)

	rr := h.doJSON(t, http.MethodPost, "/v1/sessions", fx, fx.owner, map[string]any{
		"channel_id": fx.channel.ID.String(),
		"text":       "Runtime should reject this first message",
	})
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("create session status=%d body=%s", rr.Code, rr.Body.String())
	}
	assertNoSessionCreateRows(t, h, fx.org.ID)
	if count := countActiveSessionAgentProxyTokens(t, h, fx.org.ID, fx.agent.ID, nil); count != 0 {
		t.Fatalf("active agent proxy tokens=%d, want 0", count)
	}
	if runtime.messageCalls != 1 {
		t.Fatalf("runtime message calls=%d, want 1", runtime.messageCalls)
	}
	if len(provider.deleted) != 1 {
		t.Fatalf("provider deleted=%v, want one sandbox cleanup", provider.deleted)
	}
}
