package tasks

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	slacksdk "github.com/slack-go/slack"

	"github.com/usehivy/hivy/internal/gateway"
)

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
