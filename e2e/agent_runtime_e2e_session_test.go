package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"testing"
)

type agentRuntimeMessageResponse struct {
	StreamURL string `json:"stream_url"`
	StreamID  string `json:"stream_id"`
	SessionID string `json:"session_id"`
	TraceID   string `json:"trace_id"`
	TurnID    string `json:"turn_id"`
}

func sendAgentRuntimeMessage(t *testing.T, trace *agentRuntimeE2ETrace, ctx context.Context, baseURL, token string, request map[string]any) agentRuntimeMessageResponse {
	t.Helper()
	sessionID := agentRuntimeRequestSessionID(t, request)
	body := doRuntimeJSON(
		t,
		trace,
		ctx,
		http.MethodPost,
		fmt.Sprintf("%s/sessions/%s/messages", baseURL, url.PathEscape(sessionID)),
		token,
		mustJSON(t, agentRuntimeSessionMessageBody(request)),
		http.StatusOK,
	)
	var response agentRuntimeMessageResponse
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("decode message response: %v\n%s", err, body)
	}
	if response.StreamURL == "" || response.StreamID == "" || response.SessionID == "" || response.TraceID == "" || response.TurnID == "" {
		t.Fatalf("message response missing stream/session/trace/turn fields: %s", body)
	}
	if response.SessionID != sessionID {
		t.Fatalf("message response session_id=%q want=%q body=%s", response.SessionID, sessionID, body)
	}
	wantStreamPrefix := fmt.Sprintf("/sessions/%s/streams/", sessionID)
	if len(response.StreamURL) < len(wantStreamPrefix) || response.StreamURL[:len(wantStreamPrefix)] != wantStreamPrefix {
		t.Fatalf("message response stream_url=%q want prefix=%q", response.StreamURL, wantStreamPrefix)
	}
	trace.Logf("session-api", "session_id=%s stream_id=%s trace_id=%s turn_id=%s stream_url=%s", response.SessionID, response.StreamID, response.TraceID, response.TurnID, response.StreamURL)
	return response
}

func runAgentRuntimeMessage(t *testing.T, trace *agentRuntimeE2ETrace, ctx context.Context, baseURL, token string, request map[string]any) (agentRuntimeMessageResponse, []runtimeSSEEvent, []runtimeSSEEvent) {
	t.Helper()
	response := sendAgentRuntimeMessage(t, trace, ctx, baseURL, token, request)
	events := readRuntimeSSE(t, trace, ctx, baseURL+response.StreamURL, token, nil)
	return response, events, events
}

func agentRuntimeRequestSessionID(t *testing.T, request map[string]any) string {
	t.Helper()
	if sessionID, _ := request["session_id"].(string); sessionID != "" {
		return sessionID
	}
	raw, _ := request["raw"].(map[string]any)
	if sessionID, _ := raw["test"].(string); sessionID != "" {
		return sessionID
	}
	t.Fatalf("runtime E2E request missing session_id or raw.test")
	return ""
}

func agentRuntimeSessionMessageBody(request map[string]any) map[string]any {
	body := make(map[string]any, len(request))
	for key, value := range request {
		if key == "session_id" {
			continue
		}
		body[key] = value
	}
	return body
}

func assertToolCalls(t *testing.T, events []runtimeSSEEvent, tools ...string) {
	t.Helper()
	counts := map[string]int{}
	for _, event := range events {
		if event.Name != "tool_call" {
			continue
		}
		if tool, _ := event.Payload["tool"].(string); tool != "" {
			counts[tool]++
		}
	}
	for _, tool := range tools {
		if counts[tool] == 0 {
			t.Fatalf("missing tool call %s; counts=%v events=%s", tool, counts, summarizeEvents(events))
		}
	}
}
