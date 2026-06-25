package handler_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/precontext"
	"github.com/usehivy/hivy/internal/runtimeevents"
	"github.com/usehivy/hivy/internal/tasks"
)

func TestIntegration_SessionsCreate_AlwaysOnSendsFirstMessageDirectWithoutQueueOrConfig(t *testing.T) {
	runtime := newSessionSyncRuntime(t, http.StatusOK)
	contextBuilder := &recordingPreContextBuilder{sections: []string{"## Relevant memories\n- Initial context"}}
	h, _ := newSessionRuntimeHarness(t, runtime, nil, contextBuilder)
	fx := h.seed(t)
	sb := seedAlwaysOnRuntimeSandbox(t, h, fx, runtime.server.URL, "running")

	out := h.createSession(t, fx, fx.owner, "Ship the always-on hot path")
	if out.Queued {
		t.Fatalf("queued=%t, want false for idle always-on session", out.Queued)
	}
	if out.Event == nil || out.Event.EventType != runtimeevents.EventUserMessageReceived || out.Event.Payload["text"] != "Ship the always-on hot path" {
		t.Fatalf("bad backend event: %+v", out.Event)
	}
	if out.Session.SandboxID == nil || *out.Session.SandboxID != sb.ID.String() {
		t.Fatalf("session sandbox_id=%v, want %s", out.Session.SandboxID, sb.ID)
	}
	if out.Session.AgentTurnStatus != model.SessionAgentTurnActive || out.Session.AgentTurnID == "" || out.Session.AgentStreamID == "" {
		t.Fatalf("session turn metadata missing: %+v", out.Session)
	}
	if runtime.messageCalls != 1 || runtime.lastMessageText != "Ship the always-on hot path" {
		t.Fatalf("runtime message calls=%d text=%q", runtime.messageCalls, runtime.lastMessageText)
	}
	if contextBuilder.calls != 1 {
		t.Fatalf("precontext calls=%d, want 1 for initial session", contextBuilder.calls)
	}
	if got := runtime.lastSessionContext; len(got) != 1 || got[0] != "## Relevant memories\n- Initial context" {
		t.Fatalf("runtime session_context=%#v", got)
	}
	assertRuntimeMessageKeys(t, runtime.lastMessageBody, "text", "session_context")
	if _, ok := out.Event.Payload["dynamic_context"]; ok {
		t.Fatalf("backend event payload should not include dynamic_context: %#v", out.Event.Payload)
	}
	if runtime.configCalls != 0 {
		t.Fatalf("runtime config calls=%d, want 0 for hot first message", runtime.configCalls)
	}
	if runtime.readyzCalls != 0 {
		t.Fatalf("runtime readyz calls=%d, want 0 for hot first message", runtime.readyzCalls)
	}
	assertSessionQueueCount(t, h, out.Session.ID, 0)
	assertNoSessionMessageDeliverTask(t, h)
}

func TestIntegration_SessionsCreate_AlwaysOnMissingSandboxProvisionsAndSendsFirstMessage(t *testing.T) {
	runtime := newSessionSyncRuntime(t, http.StatusOK)
	h, provider := newSessionRuntimeHarness(t, runtime, nil)
	fx := h.seed(t)

	out := h.createSession(t, fx, fx.owner, "Provision the missing always-on runtime")
	if out.Queued {
		t.Fatalf("queued=%t, want false after synchronous runtime provisioning", out.Queued)
	}
	if out.Event == nil || out.Event.EventType != runtimeevents.EventUserMessageReceived {
		t.Fatalf("bad backend event: %+v", out.Event)
	}
	if out.Session.SandboxID == nil || *out.Session.SandboxID == "" {
		t.Fatalf("session sandbox_id missing: %+v", out.Session)
	}
	if len(provider.created) != 1 {
		t.Fatalf("provider creates=%d, want 1", len(provider.created))
	}
	if runtime.messageCalls != 1 || runtime.lastMessageText != "Provision the missing always-on runtime" {
		t.Fatalf("runtime message calls=%d text=%q", runtime.messageCalls, runtime.lastMessageText)
	}
	if runtime.configCalls != 1 {
		t.Fatalf("runtime config calls=%d, want 1 sandbox-create config push", runtime.configCalls)
	}
	if out.Session.AgentTurnStatus != model.SessionAgentTurnActive || out.Session.AgentTurnID == "" || out.Session.AgentStreamID == "" {
		t.Fatalf("session turn metadata missing: %+v", out.Session)
	}
	if count := countActiveSessionAgentProxyTokens(t, h, fx.org.ID, fx.agent.ID, out.Session.SandboxID); count == 0 {
		t.Fatalf("active sandbox agent proxy tokens=%d, want at least 1", count)
	}
	assertSessionQueueCount(t, h, out.Session.ID, 0)
	assertNoSessionMessageDeliverTask(t, h)
}

func TestIntegration_SessionsSend_IdleSessionDirectSendsWithoutQueueOrConfig(t *testing.T) {
	runtime := newSessionSyncRuntime(t, http.StatusOK)
	contextBuilder := &recordingPreContextBuilder{sections: []string{"## Recent sessions\n- Existing context"}}
	h, _ := newSessionRuntimeHarness(t, runtime, nil, contextBuilder)
	fx := h.seed(t)
	seedAlwaysOnRuntimeSandbox(t, h, fx, runtime.server.URL, "running")
	created := h.createSession(t, fx, fx.owner, "First direct turn")
	releaseSessionForNextUserTurn(t, h, created.Session.ID)

	msg := h.doJSON(t, http.MethodPost, "/v1/sessions/"+created.Session.ID+"/messages", fx, fx.owner, map[string]any{
		"text": "Second direct turn",
	})
	if msg.Code != http.StatusAccepted {
		t.Fatalf("message status=%d body=%s", msg.Code, msg.Body.String())
	}
	out := decodeSessionMutation(t, msg)
	if out.Queued {
		t.Fatalf("queued=%t, want false for idle session", out.Queued)
	}
	if out.Event == nil || out.Event.EventType != runtimeevents.EventUserMessageReceived || out.Event.Payload["text"] != "Second direct turn" {
		t.Fatalf("bad backend event: %+v", out.Event)
	}
	if out.Session.AgentTurnStatus != model.SessionAgentTurnActive || out.Session.AgentTurnID == "" || out.Session.AgentStreamID == "" {
		t.Fatalf("session turn metadata missing: %+v", out.Session)
	}
	if runtime.messageCalls != 2 || runtime.lastMessageText != "Second direct turn" {
		t.Fatalf("runtime message calls=%d text=%q", runtime.messageCalls, runtime.lastMessageText)
	}
	if contextBuilder.calls != 1 {
		t.Fatalf("precontext calls=%d, want only initial session build", contextBuilder.calls)
	}
	assertRuntimeMessageKeys(t, runtime.lastMessageBody, "text")
	if runtime.configCalls != 0 {
		t.Fatalf("runtime config calls=%d, want 0 for idle direct send", runtime.configCalls)
	}
	assertSessionQueueCount(t, h, created.Session.ID, 0)
}

type recordingPreContextBuilder struct {
	calls    int
	requests []precontext.Request
	sections []string
}

func (b *recordingPreContextBuilder) Build(_ context.Context, req precontext.Request) ([]string, error) {
	b.calls++
	b.requests = append(b.requests, req)
	return append([]string(nil), b.sections...), nil
}

func assertRuntimeMessageKeys(t *testing.T, body map[string]any, want ...string) {
	t.Helper()
	if len(body) != len(want) {
		t.Fatalf("runtime body keys=%v, want %v body=%#v", mapKeys(body), want, body)
	}
	for _, key := range want {
		if _, ok := body[key]; !ok {
			t.Fatalf("runtime body missing key %q: %#v", key, body)
		}
	}
}

func mapKeys(values map[string]any) []string {
	out := make([]string, 0, len(values))
	for key := range values {
		out = append(out, key)
	}
	return out
}

func TestIntegration_SessionsSend_ActiveSessionQueuesWithoutRuntimeCallOrConfig(t *testing.T) {
	runtime := newSessionSyncRuntime(t, http.StatusOK)
	h, _ := newSessionRuntimeHarness(t, runtime, nil)
	fx := h.seed(t)
	seedAlwaysOnRuntimeSandbox(t, h, fx, runtime.server.URL, "running")
	created := h.createSession(t, fx, fx.owner, "First active turn")

	msg := h.doJSON(t, http.MethodPost, "/v1/sessions/"+created.Session.ID+"/messages", fx, fx.owner, map[string]any{
		"text": "Queue behind active turn",
	})
	if msg.Code != http.StatusAccepted {
		t.Fatalf("message status=%d body=%s", msg.Code, msg.Body.String())
	}
	out := decodeSessionMutation(t, msg)
	if !out.Queued {
		t.Fatalf("queued=%t, want true while agent turn is active", out.Queued)
	}
	if out.Event == nil || out.Event.EventType != runtimeevents.EventUserMessageReceived {
		t.Fatalf("bad backend event: %+v", out.Event)
	}
	if runtime.messageCalls != 1 {
		t.Fatalf("runtime message calls=%d, want only the initial direct send", runtime.messageCalls)
	}
	if runtime.configCalls != 0 {
		t.Fatalf("runtime config calls=%d, want 0 for active-turn queue", runtime.configCalls)
	}

	var rows []model.SessionMessageQueue
	if err := h.db.Where("session_id = ?", created.Session.ID).Order("sequence_number ASC").Find(&rows).Error; err != nil {
		t.Fatalf("load queue rows: %v", err)
	}
	if len(rows) != 1 || rows[0].SequenceNumber != 1 || rows[0].Status != "pending" || rows[0].MessageText != "Queue behind active turn" {
		t.Fatalf("queue rows=%+v, want one pending command sequence 1 row", rows)
	}
	if rows[0].SessionEventID == nil || rows[0].SessionEventID.String() != out.Event.ID {
		t.Fatalf("queue session_event_id=%v, want response event %s", rows[0].SessionEventID, out.Event.ID)
	}
	assertNoSessionMessageDeliverTask(t, h)
}

func TestIntegration_SessionsCreate_StoppedAlwaysOnSandboxSendsThroughGatewayWithoutGoWake(t *testing.T) {
	runtime := newSessionSyncRuntime(t, http.StatusOK)
	h, provider := newSessionRuntimeHarness(t, runtime, nil)
	fx := h.seed(t)
	seedAlwaysOnRuntimeSandbox(t, h, fx, runtime.server.URL, "stopped")

	out := h.createSession(t, fx, fx.owner, "Gateway should wake and send")
	if out.Queued {
		t.Fatalf("queued=%t, want false for gateway-managed direct send", out.Queued)
	}
	if len(provider.started) != 0 {
		t.Fatalf("provider started=%v, want no Go wake", provider.started)
	}
	if runtime.messageCalls != 1 {
		t.Fatalf("runtime message calls=%d, want 1", runtime.messageCalls)
	}
	if runtime.configCalls != 0 {
		t.Fatalf("runtime config calls=%d, want 0 for gateway-managed send", runtime.configCalls)
	}
	if runtime.readyzCalls != 0 {
		t.Fatalf("runtime readyz calls=%d, want 0 for gateway-managed send", runtime.readyzCalls)
	}
	assertSessionQueueCount(t, h, out.Session.ID, 0)
}

func TestIntegration_SessionsCreate_DirectDeliveryFailureReleasesTurnWithoutQueueOrConfig(t *testing.T) {
	runtime := newSessionSyncRuntime(t, http.StatusInternalServerError)
	h, _ := newSessionRuntimeHarness(t, runtime, nil)
	fx := h.seed(t)
	seedAlwaysOnRuntimeSandbox(t, h, fx, runtime.server.URL, "running")

	rr := h.doJSON(t, http.MethodPost, "/v1/sessions", fx, fx.owner, map[string]any{
		"channel_id": fx.channel.ID.String(),
		"text":       "Runtime should reject this hot send",
	})
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("create session status=%d body=%s", rr.Code, rr.Body.String())
	}
	if runtime.messageCalls != 1 {
		t.Fatalf("runtime message calls=%d, want 1", runtime.messageCalls)
	}
	if runtime.configCalls != 0 {
		t.Fatalf("runtime config calls=%d, want 0 on direct delivery failure", runtime.configCalls)
	}

	var session model.Session
	if err := h.db.Where("org_id = ?", fx.org.ID).First(&session).Error; err != nil {
		t.Fatalf("load failed session: %v", err)
	}
	if session.AgentTurnStatus != model.SessionAgentTurnIdle || session.AgentTurnLastOutcome != model.SessionAgentTurnOutcomeFailed {
		t.Fatalf("session turn not released after failure: %+v", session)
	}
	assertSessionQueueCount(t, h, session.ID.String(), 0)
	var eventCount int64
	if err := h.db.Model(&model.SessionEvent{}).Where("session_id = ?", session.ID).Count(&eventCount).Error; err != nil {
		t.Fatalf("count failed delivery events: %v", err)
	}
	if eventCount != 0 {
		t.Fatalf("failed delivery event count=%d, want 0", eventCount)
	}
}

func releaseSessionForNextUserTurn(t *testing.T, h *sessionHarness, sessionID string) {
	t.Helper()
	if err := h.db.Model(&model.Session{}).Where("id = ?", sessionID).Updates(map[string]any{
		"agent_turn_status":       model.SessionAgentTurnIdle,
		"agent_turn_id":           "",
		"agent_stream_id":         "",
		"agent_turn_started_at":   nil,
		"agent_turn_last_outcome": model.SessionAgentTurnOutcomeDone,
	}).Error; err != nil {
		t.Fatalf("release session turn: %v", err)
	}
}

func assertSessionQueueCount(t *testing.T, h *sessionHarness, sessionID string, want int64) {
	t.Helper()
	var count int64
	if err := h.db.Model(&model.SessionMessageQueue{}).Where("session_id = ?", sessionID).Count(&count).Error; err != nil {
		t.Fatalf("count session queue rows: %v", err)
	}
	if count != want {
		t.Fatalf("queue rows=%d, want %d", count, want)
	}
}

func assertNoSessionMessageDeliverTask(t *testing.T, h *sessionHarness) {
	t.Helper()
	for _, task := range h.enqueuer.Tasks() {
		if task.TypeName == tasks.TypeSessionMessageDeliver {
			t.Fatalf("unexpected %s task enqueued", tasks.TypeSessionMessageDeliver)
		}
	}
}
