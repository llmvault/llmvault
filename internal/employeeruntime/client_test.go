package employeeruntime

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewClientWithTimeoutAppliesHTTPTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	client := NewClientWithTimeout(srv.URL, "runtime-secret", time.Millisecond)

	err := client.Healthz(context.Background())
	if err == nil {
		t.Fatal("expected healthz to fail when response headers exceed configured timeout")
	}
}

// flushWriter forces each SSE write through to the client immediately so the
// reader observes the chunks at the cadence the server emits them.
func flushWrite(t *testing.T, w http.ResponseWriter, s string) {
	t.Helper()
	if _, err := io.WriteString(w, s); err != nil {
		return
	}
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

// The SSE body read must NOT be cut by the client's general Timeout: a server
// streaming far longer than it must still be read to the end via the stream client.
func TestClientStreamHTTPNotBoundByGeneralTimeout(t *testing.T) {
	const chunkDelay = 30 * time.Millisecond
	const chunks = 10 // ~300ms total, 6x the 50ms general Timeout.

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		for i := 0; i < chunks; i++ {
			time.Sleep(chunkDelay)
			flushWrite(t, w, "event: token\ndata: {\"text\":\"tok\"}\n\n")
		}
		flushWrite(t, w, "event: done\ndata: {}\n\n")
	}))
	t.Cleanup(srv.Close)

	client := NewClientWithTimeout(srv.URL, "runtime-secret", 50*time.Millisecond)

	resp, err := client.StreamHTTP(context.Background(), "gateway/http/streams/long")
	if err != nil {
		t.Fatalf("StreamHTTP: %v", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read stream body (should not be cut by the 50ms general timeout): %v", err)
	}
	got := string(raw)
	if strings.Count(got, "event: token") != chunks {
		t.Fatalf("expected %d token events, body = %q", chunks, got)
	}
	if !strings.Contains(got, "event: done") {
		t.Fatalf("expected terminal done event, body = %q", got)
	}
}

// The untimed stream client must still honor the request context: cancelling
// mid-body must unblock the reader with an error.
func TestClientStreamHTTPContextCancelStopsBody(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flushWrite(t, w, "event: token\ndata: {\"text\":\"a\"}\n\n")
		<-release // hold the body open until the test releases it.
	}))
	t.Cleanup(func() {
		close(release)
		srv.Close()
	})

	ctx, cancel := context.WithCancel(context.Background())
	client := NewClient(srv.URL, "runtime-secret")
	resp, err := client.StreamHTTP(ctx, "gateway/http/streams/cancel") //nolint:bodyclose // closed via defer below; bodyclose can't trace it past the goroutine that reads resp.Body
	if err != nil {
		t.Fatalf("StreamHTTP: %v", err)
	}
	defer resp.Body.Close()

	// Read the first event, then cancel; the read must unblock with an error.
	buf := make([]byte, 8)
	if _, err := resp.Body.Read(buf); err != nil {
		t.Fatalf("first read: %v", err)
	}
	cancel()

	done := make(chan error, 1)
	go func() {
		_, err := io.ReadAll(resp.Body)
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected a read error after context cancel, got nil")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("read did not unblock after context cancel within 2s")
	}
}

// When the server closes the connection mid-stream, the reader must see a clean
// EOF after the partial bytes rather than hanging.
func TestClientStreamHTTPDisconnectMidBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flushWrite(t, w, "event: token\ndata: {\"text\":\"partial\"}\n\n")
		// Return without a terminal event: handler exit closes the body.
	}))
	t.Cleanup(srv.Close)

	client := NewClient(srv.URL, "runtime-secret")
	resp, err := client.StreamHTTP(context.Background(), "gateway/http/streams/drop")
	if err != nil {
		t.Fatalf("StreamHTTP: %v", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read partial stream: %v", err)
	}
	got := string(raw)
	if !strings.Contains(got, "partial") {
		t.Fatalf("expected partial token before disconnect, body = %q", got)
	}
	if strings.Contains(got, "event: done") {
		t.Fatalf("did not expect a terminal event on a mid-body disconnect, body = %q", got)
	}
}

func TestClientStreamHTTPRequestsSSEWithRuntimeBearer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/gateway/http/streams/stream-1" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer runtime-secret" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Accept") != "text/event-stream" {
			t.Fatalf("accept = %q", r.Header.Get("Accept"))
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: done\ndata: {}\n\n"))
	}))
	t.Cleanup(srv.Close)

	client := NewClient(srv.URL, "runtime-secret")
	resp, err := client.StreamHTTP(context.Background(), "gateway/http/streams/stream-1")
	if err != nil {
		t.Fatalf("StreamHTTP: %v", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read stream: %v", err)
	}
	if !strings.Contains(string(raw), "event: done") {
		t.Fatalf("stream body = %q", raw)
	}
}
