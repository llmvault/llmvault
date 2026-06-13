package handler

import (
	"net/http/httptest"
	"testing"
)

// X-Stream-Token header takes precedence over the ?token= query parameter so that tokens stay out
// of access logs and Sentry breadcrumbs when clients supply the header.
func TestExtractWebStreamToken_PrefersHeaderOverQueryParam(t *testing.T) {
	req := httptest.NewRequest("GET", "/v1/agents/x/sessions/y/streams/z?token=from-query", nil)
	req.Header.Set(streamTokenHeader, "from-header")

	if got := extractWebStreamToken(req); got != "from-header" {
		t.Fatalf("extractWebStreamToken = %q, want %q (header must take precedence over query param)", got, "from-header")
	}
}

// With no header set, the ?token= query parameter is still accepted.
func TestExtractWebStreamToken_FallsBackToQueryParam(t *testing.T) {
	req := httptest.NewRequest("GET", "/v1/agents/x/sessions/y/streams/z?token=from-query", nil)

	if got := extractWebStreamToken(req); got != "from-query" {
		t.Fatalf("extractWebStreamToken = %q, want %q (query param must still work)", got, "from-query")
	}
}

// An empty string is returned when neither the header nor the query param is present.
func TestExtractWebStreamToken_EmptyWhenNeitherSet(t *testing.T) {
	req := httptest.NewRequest("GET", "/v1/agents/x/sessions/y/streams/z", nil)

	if got := extractWebStreamToken(req); got != "" {
		t.Fatalf("extractWebStreamToken = %q, want empty when neither header nor query param present", got)
	}
}
