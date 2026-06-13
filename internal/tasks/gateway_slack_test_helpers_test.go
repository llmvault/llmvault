package tasks

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/usehivy/hivy/internal/gateway"
)

type slackAPICall struct {
	Path string
	Form url.Values
}

func newGatewaySlackAPIServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(handler))
}

func recordSlackAPICall(t *testing.T, r *http.Request) slackAPICall {
	t.Helper()
	if err := r.ParseForm(); err != nil {
		t.Fatalf("parse Slack form: %v", err)
	}
	return slackAPICall{Path: r.URL.Path, Form: cloneURLValues(r.Form)}
}

func cloneURLValues(values url.Values) url.Values {
	out := make(url.Values, len(values))
	for key, val := range values {
		out[key] = append([]string(nil), val...)
	}
	return out
}

func writeSlackOK(t *testing.T, w http.ResponseWriter, ts string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	body := map[string]any{"ok": true, "channel": "C123"}
	if ts != "" {
		body["ts"] = ts
	}
	if err := json.NewEncoder(w).Encode(body); err != nil {
		t.Fatalf("write Slack response: %v", err)
	}
}

func writeSlackError(t *testing.T, w http.ResponseWriter, code string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": code}); err != nil {
		t.Fatalf("write Slack error: %v", err)
	}
}

func assertSlackMarkdownBlockText(t *testing.T, call slackAPICall, want string) {
	t.Helper()
	var blocks []map[string]any
	if err := json.Unmarshal([]byte(call.Form.Get("blocks")), &blocks); err != nil {
		t.Fatalf("decode Slack blocks: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("blocks len = %d, want 1: %#v", len(blocks), blocks)
	}
	if blocks[0]["type"] != "markdown" {
		t.Fatalf("block type = %q, want markdown", blocks[0]["type"])
	}
	if blocks[0]["text"] != want {
		t.Fatalf("markdown block text = %q, want %q", blocks[0]["text"], want)
	}
}

func gatewaySlackEvents(events ...gateway.SSEEvent) <-chan gateway.SSEEvent {
	ch := make(chan gateway.SSEEvent, len(events))
	for _, event := range events {
		ch <- event
	}
	close(ch)
	return ch
}

func gatewaySlackTestPayload() GatewaySlackPayload {
	return GatewaySlackPayload{
		ConnectionID: "conn-1",
		OrgID:        "org-1",
		AgentID:      "agent-1",
		ChannelID:    "C123",
		ThreadTS:     "1710000000.123",
		TeamID:       "T123",
		SessionID:    "session-1",
		TraceID:      "trace-1",
		TurnID:       "turn-1",
		SenderID:     "U123",
	}
}

func countSlackPath(calls []slackAPICall, path string) int {
	count := 0
	for _, call := range calls {
		if call.Path == path {
			count++
		}
	}
	return count
}

func slackCallsForPath(calls []slackAPICall, path string) []slackAPICall {
	var out []slackAPICall
	for _, call := range calls {
		if call.Path == path {
			out = append(out, call)
		}
	}
	return out
}
