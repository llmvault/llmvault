package tasks

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	slacksdk "github.com/slack-go/slack"

	"github.com/usehivy/hivy/internal/gateway"
)

func TestGatewaySlackPayloadStreamURL(t *testing.T) {
	tests := []struct {
		name    string
		payload GatewaySlackPayload
		wantURL string
	}{
		{
			name: "full stream URL used directly",
			payload: GatewaySlackPayload{
				StreamURL:  "https://example.com/gateway/http/streams/stream-123",
				RuntimeURL: "https://example.com",
			},
			wantURL: "https://example.com/gateway/http/streams/stream-123",
		},
		{
			name: "stream URL with different runtime URL",
			payload: GatewaySlackPayload{
				StreamURL:  "https://sandbox.railway.app/gateway/http/streams/abc-456",
				RuntimeURL: "https://other.railway.app",
			},
			wantURL: "https://sandbox.railway.app/gateway/http/streams/abc-456",
		},
		{
			name: "stream URL without runtime URL",
			payload: GatewaySlackPayload{
				StreamURL:  "https://standalone.example.com/gateway/http/streams/xyz-789",
				RuntimeURL: "",
			},
			wantURL: "https://standalone.example.com/gateway/http/streams/xyz-789",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.payload.StreamURL != tt.wantURL {
				t.Errorf("StreamURL = %q, want %q", tt.payload.StreamURL, tt.wantURL)
			}
		})
	}
}

func TestGatewaySlackHandler_PostsFinalThreadReplyWithoutSlackStream(t *testing.T) {
	var calls []slackAPICall
	server := newGatewaySlackAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		call := recordSlackAPICall(t, r)
		calls = append(calls, call)
		switch r.URL.Path {
		case "/assistant.threads.setStatus":
			writeSlackOK(t, w, "")
		case "/chat.postMessage":
			if call.Form.Get("thread_ts") != "1710000000.123" {
				t.Fatalf("postMessage thread_ts = %q", call.Form.Get("thread_ts"))
			}
			wantText := "I have **notion** and [docs](https://example.com)."
			if call.Form.Get("text") != wantText {
				t.Fatalf("postMessage text = %q", call.Form.Get("text"))
			}
			assertSlackMarkdownBlockText(t, call, wantText)
			writeSlackOK(t, w, "1710000002.789")
		default:
			t.Fatalf("unexpected Slack path: %s", r.URL.Path)
		}
	})
	defer server.Close()

	payload := gatewaySlackTestPayload()
	text, delivered, providerMessageID, err := (&GatewaySlackHandler{}).deliverSlackResponse(
		context.Background(),
		payload,
		slacksdk.New("xoxb-test", slacksdk.OptionAPIURL(server.URL+"/")),
		gatewaySlackEvents(
			gateway.SSEEvent{Type: "token", Data: json.RawMessage(`{"text":"I have **notion** "}`)},
			gateway.SSEEvent{Type: "token", Data: json.RawMessage(`{"text":"and [docs](https://example.com)."}`)},
			gateway.SSEEvent{Type: "final", Data: json.RawMessage(`{"text":"I have **notion** and [docs](https://example.com)."}`)},
		),
		map[string]any{},
	)
	if err != nil {
		t.Fatalf("deliver slack response: %v", err)
	}
	if !delivered || text != "I have **notion** and [docs](https://example.com)." {
		t.Fatalf("delivered=%v text=%q", delivered, text)
	}
	if providerMessageID != "1710000002.789" {
		t.Fatalf("providerMessageID = %q", providerMessageID)
	}
	if countSlackPath(calls, "/chat.startStream") != 0 ||
		countSlackPath(calls, "/chat.appendStream") != 0 ||
		countSlackPath(calls, "/chat.stopStream") != 0 {
		t.Fatalf("Slack stream methods should not be used: %#v", calls)
	}
	if countSlackPath(calls, "/chat.postMessage") != 1 {
		t.Fatalf("postMessage count = %d, want 1", countSlackPath(calls, "/chat.postMessage"))
	}
	statusCalls := slackCallsForPath(calls, "/assistant.threads.setStatus")
	if len(statusCalls) != 2 {
		t.Fatalf("status call count = %d, want 2", len(statusCalls))
	}
	if statusCalls[0].Form.Get("status") != slackAssistantStatus {
		t.Fatalf("initial status = %q, want %q", statusCalls[0].Form.Get("status"), slackAssistantStatus)
	}
	if statusCalls[1].Form.Get("status") != "" {
		t.Fatalf("clear status = %q, want empty", statusCalls[1].Form.Get("status"))
	}
}

func TestGatewaySlackHandler_PostsAccumulatedTokensOnDone(t *testing.T) {
	var calls []slackAPICall
	server := newGatewaySlackAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		call := recordSlackAPICall(t, r)
		calls = append(calls, call)
		switch r.URL.Path {
		case "/assistant.threads.setStatus":
			writeSlackOK(t, w, "")
		case "/chat.postMessage":
			if call.Form.Get("thread_ts") != "1710000000.123" {
				t.Fatalf("postMessage thread_ts = %q", call.Form.Get("thread_ts"))
			}
			if call.Form.Get("text") != "partial answer" {
				t.Fatalf("postMessage text = %q", call.Form.Get("text"))
			}
			writeSlackOK(t, w, "1710000002.789")
		default:
			t.Fatalf("unexpected Slack path: %s", r.URL.Path)
		}
	})
	defer server.Close()

	text, delivered, providerMessageID, err := (&GatewaySlackHandler{}).deliverSlackResponse(
		context.Background(),
		gatewaySlackTestPayload(),
		slacksdk.New("xoxb-test", slacksdk.OptionAPIURL(server.URL+"/")),
		gatewaySlackEvents(
			gateway.SSEEvent{Type: "token", Data: json.RawMessage(`{"text":"partial "}`)},
			gateway.SSEEvent{Type: "token", Data: json.RawMessage(`{"text":"answer"}`)},
			gateway.SSEEvent{Type: "done", Data: json.RawMessage(`{}`)},
		),
		map[string]any{},
	)
	if err != nil {
		t.Fatalf("deliver slack response: %v", err)
	}
	if !delivered || text != "partial answer" {
		t.Fatalf("delivered=%v text=%q", delivered, text)
	}
	if providerMessageID != "1710000002.789" {
		t.Fatalf("providerMessageID = %q", providerMessageID)
	}
	if countSlackPath(calls, "/chat.appendStream") != 0 || countSlackPath(calls, "/chat.stopStream") != 0 {
		t.Fatalf("stream methods should not be used: %#v", calls)
	}
	if countSlackPath(calls, "/chat.postMessage") != 1 {
		t.Fatalf("postMessage count = %d", countSlackPath(calls, "/chat.postMessage"))
	}
}

func TestGatewaySlackHandler_DoesNotPostWhenDoneHasNoVisibleResponse(t *testing.T) {
	var calls []slackAPICall
	server := newGatewaySlackAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		call := recordSlackAPICall(t, r)
		calls = append(calls, call)
		switch r.URL.Path {
		case "/assistant.threads.setStatus":
			writeSlackOK(t, w, "")
		default:
			t.Fatalf("unexpected Slack path: %s", r.URL.Path)
		}
	})
	defer server.Close()

	text, delivered, providerMessageID, err := (&GatewaySlackHandler{}).deliverSlackResponse(
		context.Background(),
		gatewaySlackTestPayload(),
		slacksdk.New("xoxb-test", slacksdk.OptionAPIURL(server.URL+"/")),
		gatewaySlackEvents(
			gateway.SSEEvent{Type: "done", Data: json.RawMessage(`{}`)},
		),
		map[string]any{},
	)
	if err != nil {
		t.Fatalf("deliver slack response: %v", err)
	}
	if delivered || text != "" || providerMessageID != "" {
		t.Fatalf("delivered=%v text=%q providerMessageID=%q", delivered, text, providerMessageID)
	}
	if countSlackPath(calls, "/chat.postMessage") != 0 {
		t.Fatalf("postMessage count = %d", countSlackPath(calls, "/chat.postMessage"))
	}
	statusCalls := slackCallsForPath(calls, "/assistant.threads.setStatus")
	if len(statusCalls) != 2 {
		t.Fatalf("status call count = %d, want 2", len(statusCalls))
	}
	if statusCalls[1].Form.Get("status") != "" {
		t.Fatalf("clear status = %q, want empty", statusCalls[1].Form.Get("status"))
	}
}

// The runtime's final event text is authoritative and must win over the
// streamed token concatenation (which can drop/duplicate tokens on the wire).
func TestGatewaySlackHandler_PrefersFinalTextOverStreamedTokens(t *testing.T) {
	wantText := "notion railway Deploy Hivy's Vercel rollbacks"
	var calls []slackAPICall
	server := newGatewaySlackAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		call := recordSlackAPICall(t, r)
		calls = append(calls, call)
		switch r.URL.Path {
		case "/assistant.threads.setStatus":
			writeSlackOK(t, w, "")
		case "/chat.postMessage":
			if call.Form.Get("text") != wantText {
				t.Fatalf("postMessage text = %q", call.Form.Get("text"))
			}
			assertSlackMarkdownBlockText(t, call, wantText)
			writeSlackOK(t, w, "1710000002.789")
		default:
			t.Fatalf("unexpected Slack path: %s", r.URL.Path)
		}
	})
	defer server.Close()

	text, delivered, _, err := (&GatewaySlackHandler{}).deliverSlackResponse(
		context.Background(),
		gatewaySlackTestPayload(),
		slacksdk.New("xoxb-test", slacksdk.OptionAPIURL(server.URL+"/")),
		gatewaySlackEvents(
			gateway.SSEEvent{Type: "token", Data: json.RawMessage(`{"text":"not"}`)},
			gateway.SSEEvent{Type: "token", Data: json.RawMessage(`{"text":"ion rail"}`)},
			gateway.SSEEvent{Type: "token", Data: json.RawMessage(`{"text":"way Deploy Hiv"}`)},
			gateway.SSEEvent{Type: "token", Data: json.RawMessage(`{"text":"y's Vercel BROKEN"}`)},
			gateway.SSEEvent{Type: "final", Data: json.RawMessage(`{"text":"notion railway Deploy Hivy's Vercel rollbacks"}`)},
		),
		map[string]any{},
	)
	if err != nil {
		t.Fatalf("deliver slack response: %v", err)
	}
	if !delivered || text != wantText {
		t.Fatalf("delivered=%v text=%q", delivered, text)
	}
}

func TestGatewaySlackHandler_PostsFriendlyMessageOnStreamError(t *testing.T) {
	var calls []slackAPICall
	server := newGatewaySlackAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		call := recordSlackAPICall(t, r)
		calls = append(calls, call)
		switch r.URL.Path {
		case "/assistant.threads.setStatus":
			writeSlackOK(t, w, "")
		case "/chat.postMessage":
			if call.Form.Get("text") != "Something went wrong. Please try again." {
				t.Fatalf("postMessage text = %q", call.Form.Get("text"))
			}
			writeSlackOK(t, w, "1710000002.789")
		default:
			t.Fatalf("unexpected Slack path: %s", r.URL.Path)
		}
	})
	defer server.Close()

	text, delivered, providerMessageID, err := (&GatewaySlackHandler{}).deliverSlackResponse(
		context.Background(),
		gatewaySlackTestPayload(),
		slacksdk.New("xoxb-test", slacksdk.OptionAPIURL(server.URL+"/")),
		gatewaySlackEvents(
			gateway.SSEEvent{Type: "error", Data: json.RawMessage(`{"message":"internal"}`)},
		),
		map[string]any{},
	)
	if err != nil {
		t.Fatalf("deliver slack response: %v", err)
	}
	if !delivered {
		t.Fatal("expected friendly error delivery")
	}
	if text != "Something went wrong. Please try again." {
		t.Fatalf("text = %q", text)
	}
	if providerMessageID != "1710000002.789" {
		t.Fatalf("providerMessageID = %q", providerMessageID)
	}
	if countSlackPath(calls, "/chat.postMessage") != 1 {
		t.Fatalf("postMessage count = %d", countSlackPath(calls, "/chat.postMessage"))
	}
}
