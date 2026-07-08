package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/billing"
	"github.com/usehivy/hivy/internal/model"
)

func TestCallQuiverImagesComputesCost(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_1","credits":20,"data":[{"svg":"<svg/>","mime_type":"image/svg+xml"}]}`))
	}))
	defer server.Close()

	h := &UploadsHandler{imageHTTPClient: server.Client()}
	images, usage, err := h.callQuiverImages(
		context.Background(),
		&model.Credential{BaseURL: server.URL, AuthScheme: "bearer"},
		"sk-test",
		"quiver-model",
		"a logo",
		"1:1",
		1,
		nil,
		false,
	)
	if err != nil {
		t.Fatalf("callQuiverImages: %v", err)
	}
	if len(images) != 1 {
		t.Fatalf("images = %d, want 1", len(images))
	}
	if usage.CreditsUsed != 20 {
		t.Fatalf("credits = %d, want 20", usage.CreditsUsed)
	}
	if usage.Cost != 0.20 {
		t.Fatalf("cost = %v, want 0.20", usage.Cost)
	}

	payload := buildModelUsagePayload(modelUsageInput{
		Operation:  "image_generation",
		OrgID:      uuid.New(),
		ProviderID: imageGenerationQuiverProviderID,
		Cost:       usage.Cost,
	})
	if payload.Generation.Cost != 0.20 {
		t.Fatalf("generation cost = %v, want 0.20", payload.Generation.Cost)
	}
	if got := billing.CostUSDToCredits(payload.Generation.Cost); got != 200 {
		t.Fatalf("credits charged = %d, want 200", got)
	}
}

func TestCallReveImagesComputesCost(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("X-Reve-Credits-Used", "30")
		_, _ = w.Write([]byte("png-bytes"))
	}))
	defer server.Close()

	h := &UploadsHandler{imageHTTPClient: server.Client()}
	images, usage, err := h.callReveImages(
		context.Background(),
		&model.Credential{BaseURL: server.URL, AuthScheme: "bearer"},
		"sk-test",
		"reve-model",
		"a poster",
		"1:1",
		1,
		nil,
	)
	if err != nil {
		t.Fatalf("callReveImages: %v", err)
	}
	if len(images) != 1 {
		t.Fatalf("images = %d, want 1", len(images))
	}
	if usage.CreditsUsed != 30 {
		t.Fatalf("credits = %d, want 30", usage.CreditsUsed)
	}
	if usage.Cost != 0.04 {
		t.Fatalf("cost = %v, want 0.04", usage.Cost)
	}
}

func TestImageGenerationZeroCreditSuccessFloored(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_2","credits":0,"data":[{"svg":"<svg/>","mime_type":"image/svg+xml"}]}`))
	}))
	defer server.Close()

	h := &UploadsHandler{imageHTTPClient: server.Client()}
	_, usage, err := h.callQuiverImages(
		context.Background(),
		&model.Credential{BaseURL: server.URL, AuthScheme: "bearer"},
		"sk-test",
		"quiver-model",
		"a logo",
		"1:1",
		1,
		nil,
		false,
	)
	if err != nil {
		t.Fatalf("callQuiverImages: %v", err)
	}
	if usage.Cost != billing.CreditUSDValue {
		t.Fatalf("cost = %v, want %v", usage.Cost, billing.CreditUSDValue)
	}
	if got := billing.CostUSDToCredits(usage.Cost); got != 1 {
		t.Fatalf("credits charged = %d, want 1", got)
	}
}
