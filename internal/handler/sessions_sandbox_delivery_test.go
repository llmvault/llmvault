package handler_test

import (
	"errors"
	"net/http"
	"testing"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/tasks"
)

func TestIntegration_SessionsCreate_SessionCreatesSandboxAndSendsFirstMessage(t *testing.T) {
	runtime := newSessionRuntimeStub(t, http.StatusOK)
	h, provider := newSessionRuntimeHarness(t, runtime, nil)
	fx := h.seed(t)

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
	if runtime.configCalls != 1 {
		t.Fatalf("runtime config calls=%d, want 1 sandbox-create config push only", runtime.configCalls)
	}
	if runtime.lastSessionID != out.Session.ID || runtime.lastMessageText != "Ship the synchronous session path" {
		t.Fatalf("runtime message session=%q text=%q", runtime.lastSessionID, runtime.lastMessageText)
	}
	var queueCount int64
	if err := h.db.Model(&model.SessionMessageQueue{}).Where("session_id = ?", out.Session.ID).Count(&queueCount).Error; err != nil {
		t.Fatalf("count queue rows: %v", err)
	}
	if queueCount != 0 {
		t.Fatalf("queue rows=%d, want 0 for direct initial delivery", queueCount)
	}
	var stored model.Session
	if err := h.db.First(&stored, "id = ?", out.Session.ID).Error; err != nil {
		t.Fatalf("load session: %v", err)
	}
	if stored.AgentTurnStatus != model.SessionAgentTurnActive || stored.AgentTurnID == "" || stored.AgentStreamID == "" {
		t.Fatalf("session turn not active after direct send: %+v", stored)
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

func TestIntegration_SessionsCreate_SessionConfigUsesSelectedModel(t *testing.T) {
	runtime := newSessionRuntimeStub(t, http.StatusOK)
	h, _ := newSessionRuntimeHarness(t, runtime, nil)
	fx := h.seed(t)

	rr := h.doJSON(t, http.MethodPost, "/v1/sessions", fx, fx.owner, map[string]any{
		"channel_id": fx.channel.ID.String(),
		"text":       "Use the selected runtime model",
		"model_definition": map[string]any{
			"model_id":         "minimax-m3",
			"reasoning_effort": "medium",
		},
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create session status=%d body=%s", rr.Code, rr.Body.String())
	}
	if runtime.lastConfigModelID != "minimax-m3" {
		t.Fatalf("runtime config model_id=%q, want minimax-m3", runtime.lastConfigModelID)
	}
	if runtime.lastConfigReasoningEffort != "medium" {
		t.Fatalf("runtime config reasoning_effort=%q, want medium", runtime.lastConfigReasoningEffort)
	}
	assertRuntimeMessageKeys(t, runtime.lastMessageBody, "text", "actor_user_id")
}

func TestIntegration_SessionsCreate_SessionSandboxFailureLeavesNoSessionRows(t *testing.T) {
	runtime := newSessionRuntimeStub(t, http.StatusOK)
	h, _ := newSessionRuntimeHarness(t, runtime, errors.New("provider unavailable"))
	fx := h.seed(t)

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

func TestIntegration_SessionsCreate_SessionFirstMessageFailureCleansUp(t *testing.T) {
	runtime := newSessionRuntimeStub(t, http.StatusInternalServerError)
	h, provider := newSessionRuntimeHarness(t, runtime, nil)
	fx := h.seed(t)

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
