package embedclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEmbedParsesUsageTotalTokens(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data": [{"embedding": [0.1, 0.2, 0.3]}],
			"usage": {"prompt_tokens": 42, "total_tokens": 42}
		}`))
	}))
	defer srv.Close()

	e := NewEmbedder(EmbedderConfig{BaseURL: srv.URL, APIKey: "k", Model: "openai/text-embedding-3-small"})
	vectors, tokens, err := e.Embed(context.Background(), []string{"hello"})
	if err != nil {
		t.Fatalf("embed: %v", err)
	}
	if len(vectors) != 1 {
		t.Fatalf("got %d vectors, want 1", len(vectors))
	}
	if tokens != 42 {
		t.Errorf("total_tokens = %d, want 42", tokens)
	}
	if e.Model() != "openai/text-embedding-3-small" {
		t.Errorf("model = %q, want openai/text-embedding-3-small", e.Model())
	}
}
