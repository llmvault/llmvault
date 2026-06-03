package tasks

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	slacksdk "github.com/slack-go/slack"

	"github.com/usehivy/hivy/internal/gateway"
	"github.com/usehivy/hivy/internal/nango"
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

func TestGatewaySlackHandler_HandleStatusSetsSlackLoading(t *testing.T) {
	nangoServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/connection/nango-conn" {
			t.Fatalf("unexpected Nango path: %s", r.URL.String())
		}
		if r.URL.Query().Get("provider_config_key") != "slack" {
			t.Fatalf("provider_config_key = %q", r.URL.Query().Get("provider_config_key"))
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"credentials": map[string]any{"bot_token": "xoxb-status"},
		}); err != nil {
			t.Fatalf("write Nango response: %v", err)
		}
	}))
	defer nangoServer.Close()

	var calls []slackAPICall
	slackServer := newGatewaySlackAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		call := recordSlackAPICall(t, r)
		calls = append(calls, call)
		if r.URL.Path != "/assistant.threads.setStatus" {
			t.Fatalf("unexpected Slack path: %s", r.URL.Path)
		}
		writeSlackOK(t, w, "")
	})
	defer slackServer.Close()

	handler := NewGatewaySlackHandler(nil, nango.NewClient(nangoServer.URL, "secret"))
	handler.slackClientFactory = func(token string) slackGatewayClient {
		if token != "xoxb-status" {
			t.Fatalf("token = %q, want xoxb-status", token)
		}
		return slacksdk.New(token, slacksdk.OptionAPIURL(slackServer.URL+"/"))
	}
	task, err := NewGatewaySlackStatusTask(GatewaySlackStatusPayload{
		ConnectionID: "conn-1",
		OrgID:        "org-1",
		EmployeeID:   "employee-1",
		ChannelID:    "C123",
		ThreadTS:     "1710000000.123",
		EventID:      "event-1",
		NangoConnID:  "nango-conn",
		ProviderKey:  "slack",
	})
	if err != nil {
		t.Fatalf("build task: %v", err)
	}

	if err := handler.HandleStatus(context.Background(), task); err != nil {
		t.Fatalf("handle status: %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("Slack calls = %d, want 1", len(calls))
	}
	if calls[0].Form.Get("channel_id") != "C123" || calls[0].Form.Get("thread_ts") != "1710000000.123" {
		t.Fatalf("status form = %#v", calls[0].Form)
	}
	if calls[0].Form.Get("status") != slackAssistantStatus {
		t.Fatalf("status = %q, want %q", calls[0].Form.Get("status"), slackAssistantStatus)
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
			if call.Form.Get("text") != "Hello there" {
				t.Fatalf("postMessage text = %q", call.Form.Get("text"))
			}
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
			gateway.SSEEvent{Type: "token", Data: json.RawMessage(`{"text":"Hello "}`)},
			gateway.SSEEvent{Type: "token", Data: json.RawMessage(`{"text":"there"}`)},
			gateway.SSEEvent{Type: "final", Data: json.RawMessage(`{"text":"Hello there"}`)},
		),
		map[string]any{},
	)
	if err != nil {
		t.Fatalf("deliver slack response: %v", err)
	}
	if !delivered || text != "Hello there" {
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

func TestGatewaySlackHandler_PostsAccumulatedTokensWhenTerminalEventMissing(t *testing.T) {
	var calls []slackAPICall
	server := newGatewaySlackAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		call := recordSlackAPICall(t, r)
		calls = append(calls, call)
		switch r.URL.Path {
		case "/assistant.threads.setStatus":
			writeSlackOK(t, w, "")
		case "/chat.postMessage":
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
			gateway.SSEEvent{Type: "token", Data: json.RawMessage(`{"text":"partial answer"}`)},
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
	if countSlackPath(calls, "/chat.startStream") != 0 ||
		countSlackPath(calls, "/chat.appendStream") != 0 ||
		countSlackPath(calls, "/chat.stopStream") != 0 {
		t.Fatalf("Slack stream methods should not be used: %#v", calls)
	}
}
