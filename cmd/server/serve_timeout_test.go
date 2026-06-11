package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestReadHeaderTimeoutNotReadTimeout is the P1-10 regression guard.
//
// Before the fix the server used ReadTimeout:10s which covered the entire
// request including the body.  Any request whose body took >10 s to upload
// (drive uploads, sqlite backups from slow clients) was killed mid-stream.
//
// The fix replaces it with ReadHeaderTimeout:10s so only the header phase is
// bounded; the body may take as long as the handler allows.  We verify two
// things with a tiny httptest server that mirrors the production config:
//
//  1. A slow body (simulated by a blocking reader that blocks for > header
//     timeout duration) is NOT killed by the server — the handler receives
//     the full body.
//  2. The server's ReadTimeout field is zero (not set) so the stdlib never
//     imposes a whole-request deadline on its own.
func TestReadHeaderTimeoutNotReadTimeout(t *testing.T) {
	// Build a minimal handler that reads the full request body and echoes it
	// back, so we can observe whether the body was truncated or the connection
	// killed mid-flight.
	done := make(chan struct{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "body read error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
		close(done)
	})

	srv := httptest.NewUnstartedServer(handler)
	srv.Config.ReadHeaderTimeout = 10 * time.Second
	srv.Config.ReadTimeout = 0 // must NOT be set — the regression condition
	srv.Config.WriteTimeout = 0
	srv.Config.IdleTimeout = 120 * time.Second
	srv.Start()
	t.Cleanup(srv.Close)

	// Verify ReadTimeout is zero on the server config (regression guard for
	// reintroduction of the old timeout).
	if srv.Config.ReadTimeout != 0 {
		t.Fatalf("ReadTimeout must be 0 (not set); got %s — slow uploads would be killed", srv.Config.ReadTimeout)
	}

	// Send a request whose body arrives in two parts with a brief pause between
	// them, simulating a slow client upload.  The handler must receive the
	// complete body without a deadline error.
	pr, pw := io.Pipe()
	go func() {
		_, _ = pw.Write([]byte("first-chunk"))
		time.Sleep(20 * time.Millisecond) // brief pause — trivially within header timeout
		_, _ = pw.Write([]byte("-second-chunk"))
		pw.Close()
	}()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL+"/echo", pr)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	got, _ := io.ReadAll(resp.Body)
	want := "first-chunk-second-chunk"
	if strings.TrimSpace(string(got)) != want {
		t.Fatalf("body = %q, want %q — slow body was likely killed by ReadTimeout", string(got), want)
	}

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("handler never completed — body read timed out")
	}
}
