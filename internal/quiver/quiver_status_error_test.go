package quiver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGenerateSVGReturnsStructuredStatusError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":       "rate_limit_exceeded",
			"message":    "Rate limit exceeded",
			"request_id": "req-error",
			"status":     429,
		})
	}))
	defer server.Close()

	client := NewClient(server.Client(), time.Second)
	_, err := client.GenerateSVG(context.Background(), GenerateRequest{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Prompt:  "an icon",
	})
	if err == nil {
		t.Fatal("GenerateSVG returned nil error")
	}
	var statusErr *StatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("error type = %T, want *StatusError", err)
	}
	if statusErr.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("StatusCode = %d", statusErr.StatusCode)
	}
	if statusErr.Code != "rate_limit_exceeded" {
		t.Fatalf("Code = %q", statusErr.Code)
	}
	if statusErr.Message != "Rate limit exceeded" {
		t.Fatalf("Message = %q", statusErr.Message)
	}
	if statusErr.RequestID != "req-error" {
		t.Fatalf("RequestID = %q", statusErr.RequestID)
	}
	if statusErr.RetryAfter != "30" {
		t.Fatalf("RetryAfter = %q", statusErr.RetryAfter)
	}
}
