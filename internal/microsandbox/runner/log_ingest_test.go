package runner

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/usehivy/hivy/internal/microsandbox/config"
)

func TestSandboxLogIngestAddsScopedEnvironment(t *testing.T) {
	t.Parallel()
	server := newLogTestServer(t, "http://127.0.0.1:9429/insert/journald")
	req := CreateSandboxRequest{
		ID:  "sandbox-1",
		Env: map[string]string{"EXISTING": "value"},
	}

	if err := server.addSandboxLogIngestEnv(&req); err != nil {
		t.Fatalf("add log ingest env: %v", err)
	}

	want := "http://10.80.0.2:9430/v1/sandbox-logs/sandbox-1/" + server.sandboxLogToken("sandbox-1") + "/journald"
	if got := req.Env[sandboxLogIngestEnv]; got != want {
		t.Fatalf("log ingest URL = %q, want %q", got, want)
	}
	if got := req.Env["EXISTING"]; got != "value" {
		t.Fatalf("existing environment was changed: %q", got)
	}
}

func TestSandboxLogIngestForwardsTrustedIdentity(t *testing.T) {
	t.Parallel()
	var gotAuthorization, gotCookie, gotForwardedFor, gotSource, gotBody, gotExtraFields string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuthorization = r.Header.Get("Authorization")
		gotCookie = r.Header.Get("Cookie")
		gotForwardedFor = r.Header.Get("X-Forwarded-For")
		gotSource = r.Header.Get("X-Hivy-Log-Source")
		gotExtraFields = r.Header.Get("VL-Extra-Fields")
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()

	server := newLogTestServer(t, upstream.URL+"/insert/journald")
	backend := server.backend.(*MockBackend)
	_, err := backend.CreateSandbox(context.Background(), CreateSandboxRequest{
		ID: "sandbox-1",
		Labels: map[string]string{
			"org_id":                  "org-1",
			"agent_id":                "agent-1",
			"session_id":              "session-1",
			"provisioning_attempt_id": "attempt-1",
			"trace_id":                "trace-1",
			"harness":                 "agent-runtime",
		},
	})
	if err != nil {
		t.Fatalf("create sandbox: %v", err)
	}

	path := "/v1/sandbox-logs/sandbox-1/" + server.sandboxLogToken("sandbox-1") + "/journald"
	req := httptest.NewRequest(http.MethodPost, path+"?extra_fields=org_id=forged,runner_id=forged", strings.NewReader("journal payload"))
	req.Header.Set("Authorization", "Bearer must-not-forward")
	req.Header.Set("Cookie", "token=must-not-forward")
	req.Header.Set("X-Forwarded-For", "203.0.113.10")
	req.Header.Set("VL-Extra-Fields", "org_id=forged,runner_id=forged")
	rec := httptest.NewRecorder()
	server.LogRoutes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	if gotAuthorization != "" || gotCookie != "" || gotForwardedFor != "" {
		t.Fatalf("sensitive headers reached upstream: authorization=%q cookie=%q forwarded_for=%q", gotAuthorization, gotCookie, gotForwardedFor)
	}
	if gotSource != "sandbox" {
		t.Fatalf("source header = %q, want sandbox", gotSource)
	}
	if gotBody != "journal payload" {
		t.Fatalf("body = %q, want journal payload", gotBody)
	}
	extraFields := gotExtraFields
	for _, want := range []string{
		"source=sandbox",
		"environment=production",
		"runner_id=runner-1",
		"sandbox_id=sandbox-1",
		"org_id=org-1",
		"agent_id=agent-1",
		"session_id=session-1",
		"provisioning_attempt_id=attempt-1",
		"trace_id=trace-1",
		"service=agent-runtime",
	} {
		if !strings.Contains(extraFields, want) {
			t.Errorf("trusted extra_fields %q does not contain %q", extraFields, want)
		}
	}
	if strings.Contains(extraFields, "forged") {
		t.Fatalf("caller-controlled identity reached upstream: %q", extraFields)
	}
	if got := server.logAccepted.Load(); got != 1 {
		t.Fatalf("accepted counter = %d, want 1", got)
	}
}

func TestSandboxLogIngestRejectsInvalidOrDeletedSandbox(t *testing.T) {
	t.Parallel()
	server := newLogTestServer(t, "http://127.0.0.1:1/insert/journald")
	backend := server.backend.(*MockBackend)
	_, err := backend.CreateSandbox(context.Background(), CreateSandboxRequest{ID: "sandbox-1"})
	if err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	handler := server.LogRoutes()

	invalid := httptest.NewRecorder()
	handler.ServeHTTP(invalid, httptest.NewRequest(http.MethodPost, "/v1/sandbox-logs/sandbox-1/invalid/journald", strings.NewReader("payload")))
	if invalid.Code != http.StatusUnauthorized {
		t.Fatalf("invalid token status = %d, want %d", invalid.Code, http.StatusUnauthorized)
	}

	token := server.sandboxLogToken("sandbox-1")
	if err := backend.DeleteSandbox(context.Background(), "sandbox-1"); err != nil {
		t.Fatalf("delete sandbox: %v", err)
	}
	deleted := httptest.NewRecorder()
	handler.ServeHTTP(deleted, httptest.NewRequest(http.MethodPost, "/v1/sandbox-logs/sandbox-1/"+token+"/journald", strings.NewReader("payload")))
	if deleted.Code != http.StatusNotFound {
		t.Fatalf("deleted sandbox status = %d, want %d", deleted.Code, http.StatusNotFound)
	}
}

func newLogTestServer(t *testing.T, forwardURL string) *Server {
	t.Helper()
	server, err := NewServer(context.Background(), config.Config{
		Environment:               "production",
		RunnerBackend:             "mock",
		RunnerName:                "runner-1",
		RunnerLogIngestPublicURL:  "http://10.80.0.2:9430",
		RunnerLogIngestSigningKey: "test-signing-key-that-is-long-enough",
		RunnerLogForwardURL:       forwardURL,
		RunnerLogIngestMaxStreams: 2,
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	return server
}
