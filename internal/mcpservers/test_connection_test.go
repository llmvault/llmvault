package mcpservers

import (
	"net/url"
	"strings"
	"testing"
)

func TestResolveLegacySSEEndpoint_RequiresSameOrigin(t *testing.T) {
	eventURL, err := url.Parse("https://mcp.example.test/sse")
	if err != nil {
		t.Fatalf("parse event URL: %v", err)
	}
	endpoint, err := resolveLegacySSEEndpoint(eventURL, "/messages?session_id=one")
	if err != nil {
		t.Fatalf("resolve same-origin endpoint: %v", err)
	}
	if endpoint != "https://mcp.example.test/messages?session_id=one" {
		t.Fatalf("endpoint = %q", endpoint)
	}
	for _, announced := range []string{
		"https://evil.example.test/messages",
		"https://mcp.example.test:8443/messages",
		"https://user@mcp.example.test/messages",
	} {
		if _, err := resolveLegacySSEEndpoint(eventURL, announced); err == nil {
			t.Fatalf("expected endpoint %q to be rejected", announced)
		}
	}
}

func TestBoundedSSEReader_ParsesEventData(t *testing.T) {
	reader := newBoundedSSEReader(strings.NewReader(": heartbeat\nevent: message\ndata: {\"one\":\ndata: true}\n\n"))
	event, data, err := reader.Next()
	if err != nil {
		t.Fatalf("read SSE event: %v", err)
	}
	if event != "message" || data != "{\"one\":\ntrue}" {
		t.Fatalf("event=%q data=%q", event, data)
	}
}
