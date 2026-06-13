package tasks

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	slacksdk "github.com/slack-go/slack"

	"github.com/usehivy/hivy/internal/nango"
)

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
	task, _, err := NewGatewaySlackStatusTask(GatewaySlackStatusPayload{
		ConnectionID: "conn-1",
		OrgID:        "org-1",
		AgentID:      "agent-1",
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
