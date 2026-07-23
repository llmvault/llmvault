package registry

import (
	"slices"
	"testing"
)

func TestOpenRouterRoutesAreRestrictedToImageModels(t *testing.T) {
	wantModelIDs := []string{
		"flux.2-klein-4b",
		"recraft-v4.1-pro-vector",
		"recraft-v4.1-vector",
		"riverflow-v2.5-fast",
		"riverflow-v2.5-pro",
	}

	var gotModelIDs []string
	for _, hivyModel := range supportedHivyModels {
		hasOpenRouter := slices.ContainsFunc(hivyModel.Routes, isOpenRouterRoute) ||
			slices.ContainsFunc(hivyModel.ProxyRoutes, isOpenRouterRoute)
		if !hasOpenRouter {
			continue
		}

		gotModelIDs = append(gotModelIDs, hivyModel.ID)
		canonical, ok := Global().CanonicalModel(hivyModel.ID)
		if !ok {
			t.Errorf("%s does not resolve", hivyModel.ID)
			continue
		}
		if !ModelSupportsImageOutput(canonical.Model) {
			t.Errorf("%s has an OpenRouter route but does not output images", hivyModel.ID)
		}
	}

	slices.Sort(gotModelIDs)
	if !slices.Equal(gotModelIDs, wantModelIDs) {
		t.Fatalf("OpenRouter canonical routes = %v, want image-only routes %v", gotModelIDs, wantModelIDs)
	}
}

func TestOpenRouterProviderMetadataContainsOnlyImageModels(t *testing.T) {
	provider, ok := Global().GetProvider("openrouter")
	if !ok {
		t.Fatal("OpenRouter provider is missing")
	}

	wantModelIDs := []string{
		"black-forest-labs/flux.2-klein-4b",
		"recraft/recraft-v4.1-pro-vector",
		"recraft/recraft-v4.1-vector",
		"sourceful/riverflow-v2.5-fast",
		"sourceful/riverflow-v2.5-pro",
	}
	gotModelIDs := make([]string, 0, len(provider.Models))
	for modelID, model := range provider.Models {
		gotModelIDs = append(gotModelIDs, modelID)
		if !ModelSupportsImageOutput(model) {
			t.Errorf("%s is a non-image model in OpenRouter provider metadata", modelID)
		}
	}

	slices.Sort(gotModelIDs)
	if !slices.Equal(gotModelIDs, wantModelIDs) {
		t.Fatalf("OpenRouter provider models = %v, want %v", gotModelIDs, wantModelIDs)
	}
}

func TestOpenRouterOnlyTextModelsAreRemoved(t *testing.T) {
	removedModelIDs := []string{
		"claude-opus-4.7-fast",
		"laguna-m.1",
		"mistral-small-4",
		"qwen3.6-flash",
		"qwen3.6-max-preview",
		"step-3.5-flash",
	}
	for _, modelID := range removedModelIDs {
		if _, ok := hivyModelsByID[modelID]; ok {
			t.Errorf("%s remains in the canonical catalog without a supported non-OpenRouter provider", modelID)
		}
	}
}

func isOpenRouterRoute(route ModelRoute) bool {
	return route.ProviderID == "openrouter"
}
