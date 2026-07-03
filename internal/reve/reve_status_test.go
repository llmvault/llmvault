package reve

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

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
