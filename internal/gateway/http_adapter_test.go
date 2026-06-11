package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/netguard"
)

// allowLoopbackForTest permits httptest's 127.0.0.1 callback servers through the
// SSRF guard for the duration of a test (the guard otherwise blocks loopback).
func allowLoopbackForTest(t *testing.T) {
	t.Helper()
	prev := netguard.AllowLoopback
	netguard.AllowLoopback = true
	t.Cleanup(func() { netguard.AllowLoopback = prev })
}

func TestHTTPAdapterDecodeJSONMarkdown(t *testing.T) {
	routeID := uuid.New()
	adapter := NewHTTPAdapter(nil)

	inbound, ok, err := adapter.DecodeInbound(t.Context(), WebhookEnvelope{
		RouteID: routeID,
		Headers: map[string]string{
			"content-type": "application/json",
		},
		Body: []byte(`{
			"markdown":"# Deploy\n\nShip this",
			"thread_id":"deploy-thread",
			"message_id":"msg-1",
			"sender_id":"user-1",
			"sender_name":"Ada",
			"callback_url":"https://gateway.test/replies"
		}`),
	})
	if err != nil {
		t.Fatalf("decode http JSON: %v", err)
	}
	if !ok {
		t.Fatalf("expected inbound message")
	}
	if inbound.Text != "# Deploy\n\nShip this" {
		t.Fatalf("markdown changed: %q", inbound.Text)
	}
	if inbound.ThreadKey != "http:"+routeID.String()+":deploy-thread" {
		t.Fatalf("thread key = %q", inbound.ThreadKey)
	}
	if inbound.DedupeKey != "http:"+routeID.String()+":msg-1" {
		t.Fatalf("dedupe key = %q", inbound.DedupeKey)
	}
}

func TestHTTPAdapterDecodeTextMarkdown(t *testing.T) {
	routeID := uuid.New()
	adapter := NewHTTPAdapter(nil)

	inbound, ok, err := adapter.DecodeInbound(t.Context(), WebhookEnvelope{
		RouteID: routeID,
		Headers: map[string]string{
			"content-type":        "text/markdown",
			"x-hivy-thread-id":    "thread-1",
			"x-hivy-message-id":   "message-1",
			"x-hivy-sender-id":    "user-1",
			"x-hivy-sender-name":  "Ada",
			"x-hivy-callback-url": "https://gateway.test/replies",
		},
		Body: []byte("**hello**\n\nworld"),
	})
	if err != nil {
		t.Fatalf("decode text markdown: %v", err)
	}
	if !ok {
		t.Fatalf("expected inbound message")
	}
	if inbound.Text != "**hello**\n\nworld" {
		t.Fatalf("markdown changed: %q", inbound.Text)
	}
	if inbound.SenderID != "user-1" || inbound.SenderName != "Ada" {
		t.Fatalf("sender metadata lost: %#v", inbound)
	}
}

func TestHTTPAdapterSendResponsePostsMarkdownCallback(t *testing.T) {
	allowLoopbackForTest(t)
	var received map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode callback: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	adapter := NewHTTPAdapter(server.Client())
	route := model.EmployeeGatewayRoute{
		ID:       uuid.New(),
		Provider: HTTPProvider,
		Config:   model.JSON{"response_url": server.URL},
	}
	payload, err := adapter.RenderResponse(t.Context(), AgentResponse{
		Route: route,
		EmployeeSession: model.EmployeeSession{
			SourceResourceKey: "http:" + route.ID.String() + ":thread-1",
		},
		ChannelID: route.ID.String(),
		ThreadID:  "thread-1",
		Text:      "## Done\n\nAll set.",
		Raw:       map[string]any{},
	})
	if err != nil {
		t.Fatalf("render response: %v", err)
	}
	handles, err := adapter.SendResponse(t.Context(), payload)
	if err != nil {
		t.Fatalf("send response: %v", err)
	}
	if len(handles) != 1 {
		t.Fatalf("handles = %d, want 1", len(handles))
	}
	if received["markdown"] != "## Done\n\nAll set." || received["text"] != "## Done\n\nAll set." {
		t.Fatalf("callback markdown changed: %#v", received)
	}
	if received["thread_id"] != "thread-1" {
		t.Fatalf("callback thread_id = %#v", received["thread_id"])
	}
}

// P1-7: callback_url is client-controlled; SendResponse must reject internal /
// cloud-metadata destinations before issuing the request (blind SSRF + response
// exfiltration). With AllowLoopback disabled (production behaviour), loopback,
// private, and metadata targets must all be refused.
func TestHTTPAdapterSendResponseRejectsSSRFCallback(t *testing.T) {
	prev := netguard.AllowLoopback
	netguard.AllowLoopback = false
	t.Cleanup(func() { netguard.AllowLoopback = prev })

	adapter := NewHTTPAdapter(nil)
	route := model.EmployeeGatewayRoute{ID: uuid.New(), Provider: HTTPProvider}

	for _, target := range []string{
		"http://169.254.169.254/latest/meta-data/",
		"http://127.0.0.1:8080/internal",
		"http://localhost/admin",
		"http://10.0.0.5/secret",
		"http://metadata.google.internal/computeMetadata/v1/",
	} {
		payload := ProviderResponsePayload{
			Route:     route,
			ChannelID: route.ID.String(),
			ThreadID:  "thread-1",
			Text:      "exfil",
			Raw:       map[string]any{"callback_url": target},
		}
		if _, err := adapter.SendResponse(t.Context(), payload); err == nil {
			t.Fatalf("expected SSRF callback %q to be rejected, but it was allowed", target)
		}
	}
}
