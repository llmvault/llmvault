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

func TestProviderHandler_CatalogModelsReturnsEntireCatalogAndProviderDetails(t *testing.T) {
	db := newModelCatalogTestDB(t)
	seedModelCatalogCredential(t, db, "atlascloud")

	req := httptest.NewRequest(http.MethodGet, "/v1/catalog/models", nil)
	rr := httptest.NewRecorder()
	handler.NewProviderHandler(registry.Global(), db).CatalogModels(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
	}

	var response struct {
		Total  int `json:"total"`
		Models []struct {
			ID        string     `json:"id"`
			NewFrom   *time.Time `json:"new_from"`
			NewTo     *time.Time `json:"new_to"`
			Providers []struct {
				ID               string         `json:"id"`
				Name             string         `json:"name"`
				DocumentationURL string         `json:"documentation_url"`
				UpstreamModelID  string         `json:"upstream_model_id"`
				Priority         int            `json:"priority"`
				Default          bool           `json:"default"`
				Available        bool           `json:"available"`
				Cost             *registry.Cost `json:"cost"`
				PricingUnit      string         `json:"pricing_unit"`
			} `json:"providers"`
		} `json:"models"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	wantTotal := len(registry.Global().CatalogModels())
	if response.Total != wantTotal || len(response.Models) != wantTotal {
		t.Fatalf("catalog total = %d with %d models, want %d",
			response.Total, len(response.Models), wantTotal)
	}

	foundImage := false
	foundDeepSeek := false
	foundLing := false
	foundEngy := false
	for _, catalogModel := range response.Models {
		switch catalogModel.ID {
		case "reve-image":
			foundImage = true
		case "deepseek-v4-pro":
			foundDeepSeek = true
			assertDeepSeekCatalogProviders(t, catalogModel.Providers)
		case "ling-3.0-flash":
			foundLing = true
			assertCatalogNewWindow(
				t,
				catalogModel.ID,
				catalogModel.NewFrom,
				catalogModel.NewTo,
				"2026-07-22T00:00:00Z",
				"2026-09-22T00:00:00Z",
			)
		case "engy-qwen3.6-35b-a3b":
			foundEngy = true
			assertCatalogNewWindow(
				t,
				catalogModel.ID,
				catalogModel.NewFrom,
				catalogModel.NewTo,
				"2026-07-24T00:00:00Z",
				"2026-09-24T00:00:00Z",
			)
		}
	}
	if !foundImage || !foundDeepSeek || !foundLing || !foundEngy {
		t.Fatalf(
			"catalog missing image=%v, DeepSeek=%v, Ling=%v, or Engy=%v",
			!foundImage,
			!foundDeepSeek,
			!foundLing,
			!foundEngy,
		)
	}
}

func assertCatalogNewWindow(
	t *testing.T,
	modelID string,
	gotFrom, gotTo *time.Time,
	wantFrom, wantTo string,
) {
	t.Helper()
	from, err := time.Parse(time.RFC3339, wantFrom)
	if err != nil {
		t.Fatalf("parse expected new_from: %v", err)
	}
	to, err := time.Parse(time.RFC3339, wantTo)
	if err != nil {
		t.Fatalf("parse expected new_to: %v", err)
	}
	if gotFrom == nil || !gotFrom.Equal(from) {
		t.Fatalf("%s new_from = %v, want %v", modelID, gotFrom, from)
	}
	if gotTo == nil || !gotTo.Equal(to) {
		t.Fatalf("%s new_to = %v, want %v", modelID, gotTo, to)
	}
}

func assertDeepSeekCatalogProviders(t *testing.T, providers []struct {
	ID               string         `json:"id"`
	Name             string         `json:"name"`
	DocumentationURL string         `json:"documentation_url"`
	UpstreamModelID  string         `json:"upstream_model_id"`
	Priority         int            `json:"priority"`
	Default          bool           `json:"default"`
	Available        bool           `json:"available"`
	Cost             *registry.Cost `json:"cost"`
	PricingUnit      string         `json:"pricing_unit"`
}) {
	t.Helper()
	if len(providers) != 2 {
		t.Fatalf("DeepSeek providers = %#v", providers)
	}
	novita := providers[0]
	if novita.ID != "novita" ||
		novita.Name != "Novita AI" ||
		novita.UpstreamModelID != "deepseek/deepseek-v4-pro" ||
		novita.Priority != 1 ||
		!novita.Default ||
		novita.Available ||
		novita.DocumentationURL == "" ||
		novita.Cost == nil ||
		novita.Cost.Input != 1.6 ||
		novita.PricingUnit != "usd_per_million_tokens" {
		t.Fatalf("Novita provider = %#v", novita)
	}
	atlas := providers[1]
	if atlas.ID != "atlascloud" ||
		atlas.UpstreamModelID != "deepseek-ai/deepseek-v4-pro" ||
		atlas.Priority != 2 ||
		atlas.Default ||
		!atlas.Available ||
		atlas.Cost == nil ||
		atlas.Cost.Input != 1.68 {
		t.Fatalf("Atlas provider = %#v", atlas)
	}
}
