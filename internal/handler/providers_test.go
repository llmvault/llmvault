package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/registry"
)

func TestProviderHandler_AllModelsReturnsCanonicalModels(t *testing.T) {
	db := newModelCatalogTestDB(t)
	seedModelCatalogCredential(t, db, "anthropic")
	seedModelCatalogCredential(t, db, "openrouter")

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	rr := httptest.NewRecorder()
	handler.NewProviderHandler(registry.Global(), db).AllModels(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
	}

	var models []struct {
		ID          string   `json:"id"`
		ProviderIDs []string `json:"provider_ids"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &models); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	foundCanonical := false
	for _, model := range models {
		if model.ID == "anthropic/claude-sonnet-4.6" {
			t.Fatalf("response exposed upstream route alias: %#v", model)
		}
		if model.ID == "flux.2-klein-4b" {
			t.Fatalf("response exposed image-only model in text model list: %#v", model)
		}
		if model.ID == "claude-sonnet-4.6" {
			foundCanonical = true
			if len(model.ProviderIDs) == 0 {
				t.Fatalf("canonical model missing provider_ids: %#v", model)
			}
		}
	}
	if !foundCanonical {
		t.Fatal("response did not include canonical claude-sonnet-4.6")
	}
}

func TestProviderHandler_AllModelsReturnsNewWindowForGPT56Models(t *testing.T) {
	db := newModelCatalogTestDB(t)
	seedModelCatalogCredential(t, db, "atlascloud")

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	rr := httptest.NewRecorder()
	handler.NewProviderHandler(registry.Global(), db).AllModels(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
	}

	var models []struct {
		ID      string     `json:"id"`
		NewFrom *time.Time `json:"new_from"`
		NewTo   *time.Time `json:"new_to"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &models); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	wantModels := map[string]bool{
		"gpt-5.6-luna":  false,
		"gpt-5.6-sol":   false,
		"gpt-5.6-terra": false,
	}
	for _, model := range models {
		if _, ok := wantModels[model.ID]; !ok {
			continue
		}
		assertCatalogNewWindow(
			t,
			model.ID,
			model.NewFrom,
			model.NewTo,
			"2026-07-23T00:00:00Z",
			"2026-09-23T00:00:00Z",
		)
		wantModels[model.ID] = true
	}
	for modelID, found := range wantModels {
		if !found {
			t.Errorf("response did not include %s", modelID)
		}
	}
}
