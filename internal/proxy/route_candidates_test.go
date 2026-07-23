package proxy

import (
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/registry"
)

func TestCandidatesForRoutesPreservesCatalogOrderAndSkipsMissingCredentials(t *testing.T) {
	atlasCloudID := uuid.New()
	routes := []registry.ModelRoute{
		{ProviderID: "atlascloud", ModelID: "deepseek-ai/deepseek-v4-flash"},
		{ProviderID: "provider-a", ModelID: "deepseek/a"},
		{ProviderID: "provider-b", ModelID: "deepseek/b"},
	}
	credentials := []model.Credential{
		{ID: atlasCloudID, ProviderID: "atlascloud"},
		{ID: uuid.New(), ProviderID: "provider-b"},
	}

	candidates := candidatesForRoutes("deepseek-v4", routes, credentials)
	if len(candidates) != 2 {
		t.Fatalf("candidate count = %d, want 2", len(candidates))
	}
	if candidates[0].ProviderID != "atlascloud" ||
		candidates[0].UpstreamID != "deepseek-ai/deepseek-v4-flash" ||
		candidates[0].CanonicalModelID != "deepseek-v4" {
		t.Fatalf("primary candidate = %#v", candidates[0])
	}
	if candidates[1].ProviderID != "provider-b" ||
		candidates[1].UpstreamID != "deepseek/b" ||
		candidates[1].CanonicalModelID != "deepseek-v4" {
		t.Fatalf("fallback candidate = %#v", candidates[1])
	}
}

func TestCandidatesForRoutesPreservesFallbackCanonicalModel(t *testing.T) {
	novitaID := uuid.New()
	routes := []registry.ModelRoute{
		{ProviderID: "xiaomi", ModelID: "mimo-v2.5-pro-ultraspeed"},
		{
			ProviderID:       "novita",
			ModelID:          "xiaomimimo/mimo-v2.5-pro",
			CanonicalModelID: "mimo-v2.5-pro",
		},
	}
	credentials := []model.Credential{
		{ID: uuid.New(), ProviderID: "xiaomi"},
		{ID: novitaID, ProviderID: "novita"},
	}

	candidates := candidatesForRoutes("mimo-v2.5-pro-ultraspeed", routes, credentials)
	if len(candidates) != 2 {
		t.Fatalf("candidate count = %d, want 2", len(candidates))
	}
	fallback := candidates[1]
	if fallback.CredentialID != novitaID.String() ||
		fallback.ProviderID != "novita" ||
		fallback.UpstreamID != "xiaomimimo/mimo-v2.5-pro" ||
		fallback.CanonicalModelID != "mimo-v2.5-pro" {
		t.Fatalf("fallback candidate = %#v", fallback)
	}
}
