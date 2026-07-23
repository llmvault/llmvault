package registry

import "testing"

func TestTogetherProviderMetadataMatchesLiveChatDirectory(t *testing.T) {
	provider, ok := Global().GetProvider("together")
	if !ok {
		t.Fatal("Together AI provider is missing")
	}
	if provider.API != "https://api.together.ai/v1" {
		t.Fatalf("API = %q", provider.API)
	}
	if got, want := len(provider.Models), 162; got != want {
		t.Fatalf("model count = %d, want %d", got, want)
	}

	for upstreamID, model := range provider.Models {
		if model.ID != upstreamID {
			t.Errorf("%s ID = %q", upstreamID, model.ID)
		}
		if model.Cost == nil ||
			model.Cost.Input < 0 ||
			model.Cost.Output < 0 ||
			model.Cost.CacheRead < 0 {
			t.Errorf("%s has invalid pricing: %#v", upstreamID, model.Cost)
		}
		if model.Modalities == nil ||
			!containsString(model.Modalities.Input, "text") ||
			!containsString(model.Modalities.Output, "text") {
			t.Errorf("%s has invalid chat modalities: %#v", upstreamID, model.Modalities)
		}

	}
}

func TestTogetherInklingMetadata(t *testing.T) {
	provider, ok := Global().GetProvider("together")
	if !ok {
		t.Fatal("Together AI provider is missing")
	}
	model, ok := provider.Models["thinkingmachines/Inkling"]
	if !ok {
		t.Fatal("Inkling is missing")
	}
	if model.Cost == nil ||
		model.Cost.Input != 1 ||
		model.Cost.Output != 4.05 ||
		model.Cost.CacheRead != 0.17 {
		t.Fatalf("cost = %#v", model.Cost)
	}
	if model.Limit == nil || model.Limit.Context != 524288 {
		t.Fatalf("limit = %#v", model.Limit)
	}
}

func TestTogetherNemotronUltraRoute(t *testing.T) {
	route, ok := Global().ResolveModel("together", "nemotron-3-ultra-550b-a55b")
	if !ok {
		t.Fatal("Nemotron Ultra Together route is missing")
	}
	if route.UpstreamID != "nvidia/nemotron-3-ultra-550b-a55b" {
		t.Fatalf("upstream = %q", route.UpstreamID)
	}
}
