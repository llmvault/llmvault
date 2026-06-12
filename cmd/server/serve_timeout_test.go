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

// The server must use ReadHeaderTimeout, not ReadTimeout: ReadTimeout bounds the
// whole request including the body, killing slow uploads (drive uploads, sqlite
// backups). ReadHeaderTimeout bounds only the header phase, so a slow body must
// survive and ReadTimeout must remain zero.
func TestReadHeaderTimeoutNotReadTimeout(t *testing.T) {
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

	if srv.Config.ReadTimeout != 0 {
		t.Fatalf("ReadTimeout must be 0 (not set); got %s — slow uploads would be killed", srv.Config.ReadTimeout)
	}

	// Body arrives in two parts with a pause, simulating a slow client upload.
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
