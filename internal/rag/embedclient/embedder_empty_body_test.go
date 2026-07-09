package embedclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestEmbedRetriesOnEmptyBody200(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Length", "0")
		w.Header().Set("X-Request-Id", "req-abc-123")
		w.Header().Set("Authorization", "Bearer upstream-echoed-it")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	e := NewEmbedder(EmbedderConfig{
		BaseURL:    srv.URL,
		APIKey:     "k",
		Model:      "openai/text-embedding-3-small",
		MaxRetries: 2,
		Timeout:    2 * time.Second,
	})

	_, _, err := e.Embed(context.Background(), []string{"hello"})
	if err == nil {
		t.Fatal("expected an error after empty-body 200s, got nil")
	}
	if !strings.Contains(err.Error(), "empty response body") {
		t.Fatalf("expected error to mention 'empty response body', got: %v", err)
	}
	if !strings.Contains(err.Error(), "status=200") {
		t.Fatalf("expected error to include status=200, got: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Fatalf("expected 3 upstream calls (1 + MaxRetries), got %d", got)
	}
}

func TestEmbedRecoversFromEmptyBody200(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			w.Header().Set("Content-Length", "0")
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data": [{"embedding": [0.1, 0.2, 0.3]}],
			"usage": {"prompt_tokens": 5, "total_tokens": 5}
		}`))
	}))
	defer srv.Close()

	e := NewEmbedder(EmbedderConfig{
		BaseURL:    srv.URL,
		APIKey:     "k",
		Model:      "openai/text-embedding-3-small",
		MaxRetries: 2,
		Timeout:    2 * time.Second,
	})

	vectors, tokens, err := e.Embed(context.Background(), []string{"hello"})
	if err != nil {
		t.Fatalf("expected eventual success after retry, got: %v", err)
	}
	if len(vectors) != 1 || len(vectors[0]) != 3 {
		t.Fatalf("unexpected vectors: %v", vectors)
	}
	if tokens != 5 {
		t.Errorf("total_tokens = %d, want 5", tokens)
	}
}

func TestEmbedEmptyBodyErrorIsRetryable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "0")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	e := NewEmbedder(EmbedderConfig{
		BaseURL:    srv.URL,
		APIKey:     "k",
		Model:      "openai/text-embedding-3-small",
		MaxRetries: 0,
		Timeout:    2 * time.Second,
	})
	_, _, err := e.Embed(context.Background(), []string{"hi"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if strings.Contains(err.Error(), "unexpected end of JSON input") {
		t.Fatalf("error still mentions JSON decode: %v", err)
	}
	if !strings.Contains(err.Error(), "empty response body") {
		t.Fatalf("top-level message not informative: %v", err)
	}
}
