package quiver

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGenerateSVGSendsRequestAndReturnsSVG(t *testing.T) {
	svgMarkup := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path d="M12 2l8 20H4z"/></svg>`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/v1/svgs/generations" {
			t.Fatalf("path = %s, want /v1/svgs/generations", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("Authorization = %q", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("Content-Type = %q", got)
		}

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if got := body["model"]; got != "arrow-1.1" {
			t.Fatalf("model = %#v", got)
		}
		if got := body["prompt"]; got != "A minimalist unicorn icon" {
			t.Fatalf("prompt = %#v", got)
		}
		if got := body["instructions"]; got != "flat monochrome style" {
			t.Fatalf("instructions = %#v", got)
		}
		if got := body["stream"]; got != false {
			t.Fatalf("stream = %#v", got)
		}
		if _, ok := body["n"]; ok {
			t.Fatalf("n should be omitted for a single output, body=%#v", body)
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-ID", "req-test")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "resp_test",
			"created": 1704067200,
			"credits": 20,
			"data": []map[string]any{
				{"mime_type": "image/svg+xml", "svg": svgMarkup},
			},
		})
	}))
	defer server.Close()

	client := NewClient(server.Client(), time.Second)
	result, err := client.GenerateSVG(context.Background(), GenerateRequest{
		APIKey:       " test-key ",
		BaseURL:      server.URL,
		Model:        "arrow-1.1",
		Prompt:       " A minimalist unicorn icon ",
		Instructions: " flat monochrome style ",
	})
	if err != nil {
		t.Fatalf("GenerateSVG returned error: %v", err)
	}
	if len(result.SVGs) != 1 {
		t.Fatalf("SVGs length = %d", len(result.SVGs))
	}
	if result.SVGs[0].Markup != svgMarkup {
		t.Fatalf("svg markup = %q", result.SVGs[0].Markup)
	}
	if result.SVGs[0].MimeType != "image/svg+xml" {
		t.Fatalf("mime type = %q", result.SVGs[0].MimeType)
	}
	if result.ResponseID != "resp_test" {
		t.Fatalf("ResponseID = %q", result.ResponseID)
	}
	if result.Credits != 20 {
		t.Fatalf("Credits = %d", result.Credits)
	}
	if result.RequestID != "req-test" {
		t.Fatalf("RequestID = %q", result.RequestID)
	}
}

func TestGenerateSVGDefaultsModelAndClampsN(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if got := body["model"]; got != DefaultModel {
			t.Fatalf("model = %#v, want %q", got, DefaultModel)
		}
		if got := body["n"]; got != float64(16) {
			t.Fatalf("n = %#v, want 16", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"svg": "<svg></svg>"}},
		})
	}))
	defer server.Close()

	client := NewClient(server.Client(), time.Second)
	result, err := client.GenerateSVG(context.Background(), GenerateRequest{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Prompt:  "an icon",
		N:       99,
	})
	if err != nil {
		t.Fatalf("GenerateSVG returned error: %v", err)
	}
	if len(result.SVGs) != 1 || result.SVGs[0].MimeType != "image/svg+xml" {
		t.Fatalf("SVGs = %#v", result.SVGs)
	}
}

func TestGenerateSVGSendsReferences(t *testing.T) {
	referenceBase64 := base64.StdEncoding.EncodeToString([]byte("reference-image"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		references, ok := body["references"].([]any)
		if !ok || len(references) != 2 {
			t.Fatalf("references = %#v", body["references"])
		}
		first := references[0].(map[string]any)
		if got := first["base64"]; got != referenceBase64 {
			t.Fatalf("references[0].base64 = %#v", got)
		}
		second := references[1].(map[string]any)
		if got := second["url"]; got != "https://example.com/logo.png" {
			t.Fatalf("references[1].url = %#v", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"svg": "<svg></svg>"}},
		})
	}))
	defer server.Close()

	client := NewClient(server.Client(), time.Second)
	_, err := client.GenerateSVG(context.Background(), GenerateRequest{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Prompt:  "a logo",
		References: []Reference{
			{Base64Data: " " + referenceBase64 + " "},
			{URL: " https://example.com/logo.png "},
		},
	})
	if err != nil {
		t.Fatalf("GenerateSVG returned error: %v", err)
	}
}

func TestGenerateSVGOmitsInstructionsWhenBlank(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if _, ok := body["instructions"]; ok {
			t.Fatalf("instructions should be omitted when blank, body=%#v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"svg": "<svg></svg>"}},
		})
	}))
	defer server.Close()

	client := NewClient(server.Client(), time.Second)
	if _, err := client.GenerateSVG(context.Background(), GenerateRequest{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Prompt:  "an icon",
	}); err != nil {
		t.Fatalf("GenerateSVG returned error: %v", err)
	}
}

func TestGenerateSVGRequiresPrompt(t *testing.T) {
	client := NewClient(nil, time.Second)
	_, err := client.GenerateSVG(context.Background(), GenerateRequest{APIKey: "test-key", Prompt: "   "})
	if err == nil || !strings.Contains(err.Error(), "prompt is required") {
		t.Fatalf("error = %v", err)
	}
}

func TestGenerateSVGRejectsInvalidBase64Reference(t *testing.T) {
	client := NewClient(nil, time.Second)
	_, err := client.GenerateSVG(context.Background(), GenerateRequest{
		APIKey:     "test-key",
		Prompt:     "an icon",
		References: []Reference{{Base64Data: "not-valid-base64!!!"}},
	})
	if err == nil || !strings.Contains(err.Error(), "invalid base64 image data") {
		t.Fatalf("error = %v", err)
	}
}

func TestVectorizeSVGSendsImage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/svgs/vectorizations" {
			t.Fatalf("path = %s, want /v1/svgs/vectorizations", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		image, ok := body["image"].(map[string]any)
		if !ok {
			t.Fatalf("image = %#v", body["image"])
		}
		if got := image["url"]; got != "https://example.com/logo.png" {
			t.Fatalf("image.url = %#v", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"credits": 15,
			"data":    []map[string]any{{"mime_type": "image/svg+xml", "svg": "<svg></svg>"}},
		})
	}))
	defer server.Close()

	client := NewClient(server.Client(), time.Second)
	result, err := client.VectorizeSVG(context.Background(), VectorizeRequest{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Image:   Reference{URL: "https://example.com/logo.png"},
	})
	if err != nil {
		t.Fatalf("VectorizeSVG returned error: %v", err)
	}
	if len(result.SVGs) != 1 || result.Credits != 15 {
		t.Fatalf("result = %#v", result)
	}
}

func TestVectorizeSVGRequiresImage(t *testing.T) {
	client := NewClient(nil, time.Second)
	_, err := client.VectorizeSVG(context.Background(), VectorizeRequest{APIKey: "test-key"})
	if err == nil || !strings.Contains(err.Error(), "requires an image") {
		t.Fatalf("error = %v", err)
	}
}
