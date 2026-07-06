package spider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestScrape_RetriesOnceOn5xx(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&attempts, 1) == 1 {
			w.WriteHeader(http.StatusBadGateway) // 502 on first attempt
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]Response{{URL: "https://x.test/p", Content: "# Hi", StatusCode: 200}})
	}))
	defer srv.Close()

	r, err := NewClient(srv.URL, "k").Scrape(context.Background(), "https://x.test/p")
	if err != nil {
		t.Fatalf("Scrape: %v", err)
	}
	if r.Content != "# Hi" {
		t.Fatalf("content = %q, want %q", r.Content, "# Hi")
	}
	if got := atomic.LoadInt32(&attempts); got != 2 {
		t.Fatalf("attempts = %d, want 2 (one retry on 5xx)", got)
	}
}

func TestScrape_Persistent5xxFailsAfterOneRetry(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	if _, err := NewClient(srv.URL, "k").Scrape(context.Background(), "https://x.test/p"); err == nil {
		t.Fatal("expected error on persistent 500")
	}
	if got := atomic.LoadInt32(&attempts); got != 2 {
		t.Fatalf("attempts = %d, want 2 (initial + one retry)", got)
	}
}

func TestScrape_NoRetryOn4xx(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	if _, err := NewClient(srv.URL, "k").Scrape(context.Background(), "https://x.test/p"); err == nil {
		t.Fatal("expected error on 400")
	}
	if got := atomic.LoadInt32(&attempts); got != 1 {
		t.Fatalf("attempts = %d, want 1 (no retry on 4xx)", got)
	}
}
