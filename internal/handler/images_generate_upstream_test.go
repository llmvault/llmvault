package handler

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/usehivy/hivy/internal/model"
)

func TestCallOpenRouterImagesReturnsUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/images" {
			t.Fatalf("path = %s, want /images", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data": [{"b64_json": "` + base64.StdEncoding.EncodeToString([]byte("image bytes")) + `"}],
			"usage": {
				"prompt_tokens": 11,
				"completion_tokens": 4175,
				"total_tokens": 4186,
				"cached_tokens": 3,
				"reasoning_tokens": 5,
				"cost": 0.04
			}
		}`))
	}))
	defer server.Close()

	h := &UploadsHandler{imageHTTPClient: server.Client()}
	images, usage, err := h.callOpenRouterImages(
		context.Background(),
		&model.Credential{BaseURL: server.URL, AuthScheme: "bearer"},
		"sk-test",
		"google/gemini-3.1-flash-image-preview",
		"a product icon",
		"1:1",
		1,
		nil,
	)
	if err != nil {
		t.Fatalf("callOpenRouterImages: %v", err)
	}
	if len(images) != 1 || string(images[0].Data) != "image bytes" {
		t.Fatalf("images = %+v", images)
	}
	if usage.PromptTokens != 11 || usage.CompletionTokens != 4175 || usage.TotalTokens != 4186 {
		t.Fatalf("usage tokens = %+v", usage)
	}
	if usage.CachedTokens != 3 || usage.ReasoningTokens != 5 || usage.Cost != 0.04 {
		t.Fatalf("usage details = %+v", usage)
	}
}
