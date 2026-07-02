package reve

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGenerateImageSendsCreateRequestAndReturnsImage(t *testing.T) {
	imageBytes := []byte("generated-webp")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/v1/image/create" {
			t.Fatalf("path = %s, want /v1/image/create", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("Authorization = %q", got)
		}
		if got := r.Header.Get("Accept"); got != string(FormatWEBP) {
			t.Fatalf("Accept = %q", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("Content-Type = %q", got)
		}

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if got := body["prompt"]; got != "A launch poster for Hivy" {
			t.Fatalf("prompt = %#v", got)
		}
		if got := body["version"]; got != "latest" {
			t.Fatalf("version = %#v", got)
		}
		if got := body["aspect_ratio"]; got != string(Aspect16x9) {
			t.Fatalf("aspect_ratio = %#v", got)
		}
		if _, ok := body["references"]; ok {
			t.Fatalf("references should be absent on create, body=%#v", body)
		}
		if _, ok := body["reference_images"]; ok {
			t.Fatalf("reference_images should be absent on create, body=%#v", body)
		}
		postprocessing := body["postprocessing"].([]any)
		firstPostprocess := postprocessing[0].(map[string]any)
		if got := firstPostprocess["process"]; got != "upscale" {
			t.Fatalf("postprocessing process = %#v", got)
		}
		if got := firstPostprocess["upscale_factor"]; got != float64(2) {
			t.Fatalf("upscale_factor = %#v", got)
		}

		w.Header().Set("Content-Type", string(FormatWEBP))
		w.Header().Set("X-Reve-Version", "latest")
		w.Header().Set("X-Reve-Content-Violation", "false")
		w.Header().Set("X-Reve-Request-Id", "rsid-test")
		w.Header().Set("X-Reve-Credits-Used", "150")
		w.Header().Set("X-Reve-Credits-Remaining", "850")
		_, _ = w.Write(imageBytes)
	}))
	defer server.Close()

	client := NewClient(server.Client(), time.Second)
	result, err := client.GenerateImage(context.Background(), GenerateRequest{
		APIKey:         " test-key ",
		BaseURL:        server.URL,
		Instruction:    " A launch poster for Hivy ",
		AspectRatio:    Aspect16x9,
		Format:         FormatWEBP,
		Postprocessing: []PostprocessOperation{Upscale(2), RemoveBackground()},
	})
	if err != nil {
		t.Fatalf("GenerateImage returned error: %v", err)
	}
	if string(result.Data) != string(imageBytes) {
		t.Fatalf("image data = %q", result.Data)
	}
	if result.ContentType != string(FormatWEBP) {
		t.Fatalf("ContentType = %q", result.ContentType)
	}
	if result.RequestID != "rsid-test" {
		t.Fatalf("RequestID = %q", result.RequestID)
	}
	if result.CreditsUsed != 150 || result.CreditsRemaining != 850 {
		t.Fatalf("credits = used %d remaining %d", result.CreditsUsed, result.CreditsRemaining)
	}
	if result.ContentViolation {
		t.Fatal("ContentViolation = true")
	}
}

func TestGenerateImageWithReferencesSendsRemixRequest(t *testing.T) {
	imageBytes := []byte("generated-png")
	referenceBytes := []byte("reference-image")
	referenceBase64 := base64.StdEncoding.EncodeToString([]byte("second-reference"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/v1/image/remix" {
			t.Fatalf("path = %s, want /v1/image/remix", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("Authorization = %q", got)
		}
		if got := r.Header.Get("Accept"); got != string(FormatPNG) {
			t.Fatalf("Accept = %q", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("Content-Type = %q", got)
		}

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if got := body["prompt"]; got != "A launch poster for Hivy" {
			t.Fatalf("prompt = %#v", got)
		}
		if got := body["version"]; got != "latest" {
			t.Fatalf("version = %#v", got)
		}
		if got := body["aspect_ratio"]; got != string(Aspect1x1) {
			t.Fatalf("aspect_ratio = %#v", got)
		}
		if _, ok := body["references"]; ok {
			t.Fatalf("references should be absent on remix, body=%#v", body)
		}
		if _, ok := body["postprocessing"]; ok {
			t.Fatalf("postprocessing should be absent on remix, body=%#v", body)
		}
		if _, ok := body["layout"]; ok {
			t.Fatalf("layout should be absent on remix, body=%#v", body)
		}
		referenceImages := body["reference_images"].([]any)
		if len(referenceImages) != 2 {
			t.Fatalf("reference_images length = %d", len(referenceImages))
		}
		if got := referenceImages[0]; got != base64.StdEncoding.EncodeToString(referenceBytes) {
			t.Fatalf("reference_images[0] = %#v", got)
		}
		if got := referenceImages[1]; got != referenceBase64 {
			t.Fatalf("reference_images[1] = %#v", got)
		}

		w.Header().Set("Content-Type", string(FormatPNG))
		w.Header().Set("X-Reve-Request-Id", "rsid-remix")
		_, _ = w.Write(imageBytes)
	}))
	defer server.Close()

	client := NewClient(server.Client(), time.Second)
	result, err := client.GenerateImage(context.Background(), GenerateRequest{
		APIKey:      " test-key ",
		BaseURL:     server.URL,
		Instruction: " A launch poster for Hivy ",
		AspectRatio: Aspect1x1,
		References: []Reference{
			{Data: referenceBytes, Prompt: " brand palette "},
			{Base64Data: " " + referenceBase64 + " "},
		},
	})
	if err != nil {
		t.Fatalf("GenerateImage returned error: %v", err)
	}
	if string(result.Data) != string(imageBytes) {
		t.Fatalf("image data = %q", result.Data)
	}
	if result.RequestID != "rsid-remix" {
		t.Fatalf("RequestID = %q", result.RequestID)
	}
}

func TestGenerateImageRejectsURLReferenceForRemix(t *testing.T) {
	client := NewClient(nil, time.Second)
	_, err := client.GenerateImage(context.Background(), GenerateRequest{
		APIKey:      "test-key",
		Instruction: "A poster",
		References:  []Reference{{Ref: "https://example.com/image.png"}},
	})
	if err == nil {
		t.Fatal("GenerateImage returned nil error")
	}
	if !strings.Contains(err.Error(), "reve remix requires image data references, got url reference") {
		t.Fatalf("error = %q", err)
	}
}

func TestGenerateImageRejectsPostprocessingWithReferences(t *testing.T) {
	client := NewClient(nil, time.Second)
	_, err := client.GenerateImage(context.Background(), GenerateRequest{
		APIKey:         "test-key",
		Instruction:    "A poster",
		References:     []Reference{{Data: []byte("reference-image")}},
		Postprocessing: []PostprocessOperation{Upscale(2)},
	})
	if err == nil {
		t.Fatal("GenerateImage returned nil error")
	}
	if !strings.Contains(err.Error(), "reve remix does not support postprocessing/layout") {
		t.Fatalf("error = %q", err)
	}
}

func TestGenerateImageRejectsLayoutWithReferences(t *testing.T) {
	client := NewClient(nil, time.Second)
	_, err := client.GenerateImage(context.Background(), GenerateRequest{
		APIKey:      "test-key",
		Instruction: "A poster",
		References:  []Reference{{Data: []byte("reference-image")}},
		Layout:      map[string]any{"prompt": "poster"},
	})
	if err == nil {
		t.Fatal("GenerateImage returned nil error")
	}
	if !strings.Contains(err.Error(), "reve remix does not support postprocessing/layout") {
		t.Fatalf("error = %q", err)
	}
}

func TestGenerateImageRejectsInvalidBase64Reference(t *testing.T) {
	client := NewClient(nil, time.Second)
	_, err := client.GenerateImage(context.Background(), GenerateRequest{
		APIKey:      "test-key",
		Instruction: "A poster",
		References:  []Reference{{Base64Data: "not-valid-base64!!!"}},
	})
	if err == nil {
		t.Fatal("GenerateImage returned nil error")
	}
	if !strings.Contains(err.Error(), "invalid base64 image data") {
		t.Fatalf("error = %q", err)
	}
}

func TestGenerateImageRemixReturnsStructuredStatusError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/image/remix" {
			t.Fatalf("path = %s, want /v1/image/remix", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Reve-Request-Id", "rsid-remix-error")
		w.Header().Set("X-Reve-Error-Code", "UNRECOGNIZED_PARAMETER")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error_code": "UNRECOGNIZED_PARAMETER",
			"message":    "Unrecognized parameter",
		})
	}))
	defer server.Close()

	client := NewClient(server.Client(), time.Second)
	_, err := client.GenerateImage(context.Background(), GenerateRequest{
		APIKey:      "test-key",
		BaseURL:     server.URL,
		Instruction: "A poster",
		References:  []Reference{{Data: []byte("reference-image")}},
	})
	if err == nil {
		t.Fatal("GenerateImage returned nil error")
	}
	var statusErr *StatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("error type = %T, want *StatusError", err)
	}
	if statusErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("StatusCode = %d", statusErr.StatusCode)
	}
	if statusErr.RequestID != "rsid-remix-error" {
		t.Fatalf("RequestID = %q", statusErr.RequestID)
	}
	if statusErr.ErrorCode != "UNRECOGNIZED_PARAMETER" {
		t.Fatalf("ErrorCode = %q", statusErr.ErrorCode)
	}
}

func TestGenerateImageDecodesJSONResponse(t *testing.T) {
	imageBytes := []byte("generated-png")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"image":             base64.StdEncoding.EncodeToString(imageBytes),
			"layout":            map[string]any{"prompt": "poster"},
			"version":           "latest",
			"content_violation": false,
			"request_id":        "rsid-json",
			"credits_used":      150,
			"credits_remaining": 700,
		})
	}))
	defer server.Close()

	client := NewClient(server.Client(), time.Second)
	result, err := client.GenerateImage(context.Background(), GenerateRequest{
		APIKey:      "test-key",
		BaseURL:     server.URL,
		Instruction: "A poster",
	})
	if err != nil {
		t.Fatalf("GenerateImage returned error: %v", err)
	}
	if string(result.Data) != string(imageBytes) {
		t.Fatalf("image data = %q", result.Data)
	}
	if result.ContentType != string(FormatPNG) {
		t.Fatalf("ContentType = %q", result.ContentType)
	}
	if len(result.Layout) == 0 {
		t.Fatal("Layout is empty")
	}
	if result.RequestID != "rsid-json" || result.CreditsRemaining != 700 {
		t.Fatalf("metadata = request %q remaining %d", result.RequestID, result.CreditsRemaining)
	}
}

func TestGenerateImageOmitsBlankAspectRatio(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if _, ok := body["aspect_ratio"]; ok {
			t.Fatalf("aspect_ratio should be omitted when blank, body=%#v", body)
		}
		w.Header().Set("Content-Type", string(FormatPNG))
		_, _ = w.Write([]byte("generated-png"))
	}))
	defer server.Close()

	client := NewClient(server.Client(), time.Second)
	result, err := client.GenerateImage(context.Background(), GenerateRequest{
		APIKey:      "test-key",
		BaseURL:     server.URL,
		Instruction: "A poster",
	})
	if err != nil {
		t.Fatalf("GenerateImage returned error: %v", err)
	}
	if string(result.Data) != "generated-png" {
		t.Fatalf("image data = %q", result.Data)
	}
}

func TestGenerateImageReturnsStructuredStatusError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Reve-Request-Id", "rsid-error")
		w.Header().Set("X-Reve-Error-Code", "MISSING_REQUIRED_PARAMETER")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error_code": "MISSING_REQUIRED_PARAMETER",
			"message":    "Missing one or more required parameters",
			"params": map[string]any{
				"missing": []string{"prompt"},
			},
		})
	}))
	defer server.Close()

	client := NewClient(server.Client(), time.Second)
	_, err := client.GenerateImage(context.Background(), GenerateRequest{
		APIKey:      "test-key",
		BaseURL:     server.URL,
		Instruction: "A poster",
	})
	if err == nil {
		t.Fatal("GenerateImage returned nil error")
	}
	var statusErr *StatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("error type = %T, want *StatusError", err)
	}
	if statusErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("StatusCode = %d", statusErr.StatusCode)
	}
	if statusErr.RequestID != "rsid-error" {
		t.Fatalf("RequestID = %q", statusErr.RequestID)
	}
	if statusErr.ErrorCode != "MISSING_REQUIRED_PARAMETER" {
		t.Fatalf("ErrorCode = %q", statusErr.ErrorCode)
	}
	if statusErr.Message != "Missing one or more required parameters" {
		t.Fatalf("Message = %q", statusErr.Message)
	}
	if statusErr.Params["missing"] == nil {
		t.Fatalf("Params = %#v", statusErr.Params)
	}
}
