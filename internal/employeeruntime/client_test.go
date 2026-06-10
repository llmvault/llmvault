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
