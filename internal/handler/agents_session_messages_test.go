package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/auth"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

func (h *agentHarness) sendSessionMessage(t *testing.T, m orgWithMember, agentID uuid.UUID, body any) *httptest.ResponseRecorder {
	t.Helper()
	buf := new(bytes.Buffer)
	_ = json.NewEncoder(buf).Encode(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/agents/"+agentID.String()+"/sessions/messages", buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-ID", m.org.ID.String())
	req = middleware.WithAuthClaims(req, &auth.AuthClaims{
		UserID: m.user.ID.String(),
		OrgID:  m.org.ID.String(),
		Role:   "member",
	})
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

func (h *agentHarness) streamSession(t *testing.T, m orgWithMember, streamURL string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, streamURL, nil)
	req.Header.Set("X-Org-ID", m.org.ID.String())
	req = middleware.WithAuthClaims(req, &auth.AuthClaims{
		UserID: m.user.ID.String(),
		OrgID:  m.org.ID.String(),
		Role:   "member",
	})
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

func TestAgentHandler_SendSessionMessageCreatesWebSessionAndRuntimeTurn(t *testing.T) {
	h := newAgentHarness(t)
	m := h.createOrgWithRole(t, "member")
	agent := h.seedAgentAgent(t, m)
	h.seedSandbox(t, m, agent.ID)

	rr := h.sendSessionMessage(t, m, agent.ID, map[string]any{"text": "Can you inspect the deploy?"})
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var out struct {
		AgentSessionID          string `json:"agent_session_id"`
		RuntimeSessionID        string `json:"runtime_session_id"`
		RuntimeStreamID         string `json:"runtime_stream_id"`
		RuntimeResponseID       string `json:"runtime_response_stream_id"`
		ResponseStreamURL       string `json:"response_stream_url"`
		DirectResponseStreamURL string `json:"direct_response_stream_url"`
		Created                 bool   `json:"created"`
		Source                  string `json:"source"`
		RuntimeConversationID   string `json:"runtime_conversation_id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !out.Created || out.Source != "web" {
		t.Fatalf("created/source = %v/%q", out.Created, out.Source)
	}
	if out.AgentSessionID == "" || out.RuntimeSessionID == "" || out.ResponseStreamURL == "" {
		t.Fatalf("missing response fields: %#v", out)
	}
	if !strings.Contains(out.ResponseStreamURL, "/v1/agents/"+agent.ID.String()+"/sessions/"+out.AgentSessionID+"/streams/"+out.RuntimeResponseID) {
		t.Fatalf("response stream url = %q", out.ResponseStreamURL)
	}
	if !strings.Contains(out.DirectResponseStreamURL, "/gateway/http/response-streams/"+out.RuntimeResponseID) {
		t.Fatalf("direct response stream url = %q", out.DirectResponseStreamURL)
	}
	directURL, err := url.Parse(out.DirectResponseStreamURL)
	if err != nil {
		t.Fatalf("parse direct response stream url: %v", err)
	}
	if directURL.Query().Get("stream_token") == "" {
		t.Fatalf("direct response stream url missing stream_token: %q", out.DirectResponseStreamURL)
	}

	var session model.AgentSession
	if err := h.db.First(&session, "id = ?", out.AgentSessionID).Error; err != nil {
		t.Fatalf("load session: %v", err)
	}
	if session.Source != "web" || session.AgentID != agent.ID || session.OrgID != m.org.ID {
		t.Fatalf("stored session = %#v", session)
	}
	if session.RuntimeConversationID != out.RuntimeSessionID {
		t.Fatalf("runtime conversation id = %q, response session = %q", session.RuntimeConversationID, out.RuntimeSessionID)
	}

	calls, bearer, body := h.sidecar.snapshotHTTPMessage()
	if calls != 1 {
		t.Fatalf("runtime message calls = %d", calls)
	}
	if bearer == "" {
		t.Fatal("runtime message missing bearer")
	}
	var runtimeReq map[string]any
	if err := json.Unmarshal(body, &runtimeReq); err != nil {
		t.Fatalf("decode runtime request: %v", err)
	}
	if runtimeReq["text"] != "Can you inspect the deploy?" {
		t.Fatalf("runtime text = %#v", runtimeReq["text"])
	}
	if runtimeReq["conversation_id"] != strings.TrimPrefix(session.RuntimeConversationID, "http-") {
		t.Fatalf("runtime conversation = %#v, session = %q", runtimeReq["conversation_id"], session.RuntimeConversationID)
	}
	raw, ok := runtimeReq["raw"].(map[string]any)
	if !ok || raw["source"] != "web" || raw["agent_session_id"] != session.ID.String() {
		t.Fatalf("runtime raw = %#v", runtimeReq["raw"])
	}
}

func TestAgentHandler_SendSessionMessageUsesLatestAgentRuntime(t *testing.T) {
	h := newAgentHarness(t)
	m := h.createOrgWithRole(t, "member")
	agent := h.seedAgentAgent(t, m)
	_ = h.seedSandbox(t, m, agent.ID)
	latestSandbox := h.seedSandbox(t, m, agent.ID)

	rr := h.sendSessionMessage(t, m, agent.ID, map[string]any{"text": "Start from the web"})
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var out struct {
		AgentSessionID string `json:"agent_session_id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	var session model.AgentSession
	if err := h.db.First(&session, "id = ?", out.AgentSessionID).Error; err != nil {
		t.Fatalf("load session: %v", err)
	}
	if session.SandboxID != latestSandbox.ID {
		t.Fatalf("session sandbox = %s, want latest runtime %s", session.SandboxID, latestSandbox.ID)
	}
}

func TestAgentHandler_SendSessionMessageContinuesExistingWebSession(t *testing.T) {
	h := newAgentHarness(t)
	m := h.createOrgWithRole(t, "member")
	agent := h.seedAgentAgent(t, m)
	h.seedSandbox(t, m, agent.ID)

	first := h.sendSessionMessage(t, m, agent.ID, map[string]any{"text": "First"})
	if first.Code != http.StatusAccepted {
		t.Fatalf("first status = %d, body = %s", first.Code, first.Body.String())
	}
	var firstOut struct {
		AgentSessionID string `json:"agent_session_id"`
	}
	_ = json.Unmarshal(first.Body.Bytes(), &firstOut)

	second := h.sendSessionMessage(t, m, agent.ID, map[string]any{
		"session_id": firstOut.AgentSessionID,
		"text":       "Second",
	})
	if second.Code != http.StatusAccepted {
		t.Fatalf("second status = %d, body = %s", second.Code, second.Body.String())
	}
	var secondOut struct {
		AgentSessionID string `json:"agent_session_id"`
		Created        bool   `json:"created"`
	}
	_ = json.Unmarshal(second.Body.Bytes(), &secondOut)
	if secondOut.Created || secondOut.AgentSessionID != firstOut.AgentSessionID {
		t.Fatalf("second response = %#v, first = %#v", secondOut, firstOut)
	}

	var count int64
	h.db.Model(&model.AgentSession{}).Where("org_id = ? AND agent_id = ? AND source = ?", m.org.ID, agent.ID, "web").Count(&count)
	if count != 1 {
		t.Fatalf("web session count = %d", count)
	}
	calls, _, body := h.sidecar.snapshotHTTPMessage()
	if calls != 2 {
		t.Fatalf("runtime message calls = %d", calls)
	}
	var runtimeReq map[string]any
	_ = json.Unmarshal(body, &runtimeReq)
	if runtimeReq["text"] != "Second" {
		t.Fatalf("runtime text = %#v", runtimeReq["text"])
	}
}

func TestAgentHandler_StreamSessionProxiesSignedRuntimeSSE(t *testing.T) {
	h := newAgentHarness(t)
	m := h.createOrgWithRole(t, "member")
	agent := h.seedAgentAgent(t, m)
	h.seedSandbox(t, m, agent.ID)
	h.sidecar.mu.Lock()
	h.sidecar.httpStreamBody = "event: token\ndata: {\"text\":\"streamed\"}\n\nevent: done\ndata: {\"session_id\":\"http-web\"}\n\n"
	h.sidecar.mu.Unlock()

	send := h.sendSessionMessage(t, m, agent.ID, map[string]any{"text": "Stream this"})
	if send.Code != http.StatusAccepted {
		t.Fatalf("send status = %d, body = %s", send.Code, send.Body.String())
	}
	var out struct {
		ResponseStreamURL string `json:"response_stream_url"`
	}
	_ = json.Unmarshal(send.Body.Bytes(), &out)

	stream := h.streamSession(t, m, out.ResponseStreamURL)
	if stream.Code != http.StatusOK {
		t.Fatalf("stream status = %d, body = %s", stream.Code, stream.Body.String())
	}
	if !strings.Contains(stream.Header().Get("Content-Type"), "text/event-stream") {
		t.Fatalf("content-type = %q", stream.Header().Get("Content-Type"))
	}
	if got := stream.Header().Get("Cache-Control"); got != "no-cache, no-transform" {
		t.Fatalf("cache-control = %q", got)
	}
	if got := stream.Header().Get("X-Accel-Buffering"); got != "no" {
		t.Fatalf("x-accel-buffering = %q", got)
	}
	if !strings.Contains(stream.Body.String(), "event: token") || !strings.Contains(stream.Body.String(), "streamed") {
		t.Fatalf("stream body = %q", stream.Body.String())
	}
}
