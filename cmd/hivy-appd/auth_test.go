package main

import (
	"net/http"
	"testing"
)

func TestAllEndpointsRequireBearerAuth(t *testing.T) {
	_, ts := newTestServer(t)
	endpoints := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/deploy"},
		{http.MethodPost, "/rollback"},
		{http.MethodGet, "/logs"},
		{http.MethodGet, "/health"},
		{http.MethodPost, "/env"},
	}
	for _, ep := range endpoints {
		status, body := doJSON(t, ts, ep.method, ep.path, "", map[string]any{})
		if status != http.StatusUnauthorized {
			t.Errorf("%s %s without bearer: status = %d, want 401", ep.method, ep.path, status)
		}
		if body["error"] != "missing authorization" {
			t.Errorf("%s %s: error = %v", ep.method, ep.path, body["error"])
		}

		status, body = doJSON(t, ts, ep.method, ep.path, "wrong-secret", map[string]any{})
		if status != http.StatusUnauthorized {
			t.Errorf("%s %s with wrong bearer: status = %d, want 401", ep.method, ep.path, status)
		}
		if body["error"] != "invalid app secret" {
			t.Errorf("%s %s: error = %v", ep.method, ep.path, body["error"])
		}
	}
}

func TestCorrectBearerIsAccepted(t *testing.T) {
	_, ts := newTestServer(t)
	status, _ := doJSON(t, ts, http.MethodGet, "/health", testSecret, nil)
	if status != http.StatusOK {
		t.Fatalf("health with correct bearer: status = %d, want 200", status)
	}
}

func TestBearerFromHeader(t *testing.T) {
	cases := map[string]string{
		"Bearer abc":  "abc",
		"bearer abc":  "abc",
		"Bearer  abc": "abc",
		"Basic abc":   "",
		"Bearer":      "",
		"":            "",
	}
	for header, want := range cases {
		if got := bearerFromHeader(header); got != want {
			t.Errorf("bearerFromHeader(%q) = %q, want %q", header, got, want)
		}
	}
}
