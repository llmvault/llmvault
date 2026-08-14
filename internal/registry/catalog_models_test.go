package registry

import (
	"slices"
	"testing"
	"time"
)

func TestCatalogModelsReturnsEveryCanonicalModelWithOrderedProviderRoutes(t *testing.T) {
	models := Global().CatalogModels()
	if len(models) != len(supportedHivyModels) {
		t.Fatalf("catalog model count = %d, want %d", len(models), len(supportedHivyModels))
	}

	for i, model := range models {
		if i > 0 && models[i-1].ID >= model.ID {
			t.Fatalf("catalog is not sorted at %q and %q", models[i-1].ID, model.ID)
		}
		if len(model.Routes) == 0 {
			t.Fatalf("%s has no provider routes", model.ID)
		}
	}

	deepseek := catalogModelByID(t, models, "deepseek-v4-pro")
	if got, want := len(deepseek.Routes), 3; got != want {
		t.Fatalf("DeepSeek V4 Pro route count = %d, want %d", got, want)
	}
	direct := deepseek.Routes[0]
	if direct.ProviderID != "deepseek" ||
		direct.UpstreamModelID != "deepseek-v4-pro" ||
		direct.Model.ReleaseDate != "2026-08-13" {
		t.Fatalf("primary route = %#v", direct)
	}
	novita := deepseek.Routes[1]
	if novita.ProviderID != "novita" ||
		novita.UpstreamModelID != "deepseek/deepseek-v4-pro" ||
		novita.Model.Cost == nil ||
		novita.Model.Cost.Input != 1.6 {
		t.Fatalf("primary route = %#v", novita)
	}
	atlas := deepseek.Routes[2]
	if atlas.ProviderID != "atlascloud" ||
		atlas.UpstreamModelID != "deepseek-ai/deepseek-v4-pro" ||
		atlas.Model.Cost == nil ||
		atlas.Model.Cost.Input != 1.68 {
		t.Fatalf("fallback route = %#v", atlas)
	}

	image := catalogModelByID(t, models, "reve-image")
	if len(image.Routes) != 1 || !ModelSupportsImageOutput(image.Model) {
		t.Fatalf("image catalog entry = %#v", image)
	}

	ling := catalogModelByID(t, models, "ling-3.0-flash")
	assertNewWindow(
		t,
		ling.Model,
		"2026-07-22T00:00:00Z",
		"2026-09-22T00:00:00Z",
	)

	engy := catalogModelByID(t, models, "engy-qwen3.6-35b-a3b")
	assertNewWindow(
		t,
		engy.Model,
		"2026-07-24T00:00:00Z",
		"2026-09-24T00:00:00Z",
	)
}

func catalogModelByID(t *testing.T, models []CatalogModel, id string) CatalogModel {
	t.Helper()
	for _, model := range models {
		if model.ID == id {
			return model
		}
	}
	t.Fatalf("catalog model %q not found", id)
	return CatalogModel{}
}

func assertNewWindow(t *testing.T, model Model, wantFrom, wantTo string) {
	t.Helper()
	from, err := time.Parse(time.RFC3339, wantFrom)
	if err != nil {
		t.Fatalf("parse expected new_from: %v", err)
	}
	to, err := time.Parse(time.RFC3339, wantTo)
	if err != nil {
		t.Fatalf("parse expected new_to: %v", err)
	}
	if model.NewFrom == nil || !model.NewFrom.Equal(from) {
		t.Fatalf("%s new_from = %v, want %v", model.ID, model.NewFrom, from)
	}
	if model.NewTo == nil || !model.NewTo.Equal(to) {
		t.Fatalf("%s new_to = %v, want %v", model.ID, model.NewTo, to)
	}
}

func TestCatalogNewModelsAtReferenceTime(t *testing.T) {
	now := time.Date(2026, time.July, 24, 12, 0, 0, 0, time.UTC)
	active := make([]string, 0)
	for _, model := range Global().CatalogModels() {
		if model.NewFrom == nil || model.NewTo == nil {
			continue
		}
		if !now.Before(*model.NewFrom) && now.Before(*model.NewTo) {
			active = append(active, model.ID)
		}
	}
	slices.Sort(active)
	want := []string{
		"agent-max",
		"agent-prime",
		"agent-standard",
		"bytedance-pro-latest",
		"claude-opus-latest",
		"claude-sonnet-5",
		"cobuddy",
		"code-max",
		"code-prime",
		"code-standard",
		"deepseek-pro-latest",
		"engy-glm-5.2",
		"engy-qwen3.6-35b-a3b",
		"gemini-pro-latest",
		"glm-5.2",
		"glm-latest",
		"gpt-5.6-luna",
		"gpt-5.6-sol",
		"gpt-5.6-terra",
		"gpt-sol-latest",
		"hy3",
		"kimi-k2.7-code",
		"kimi-k3",
		"kimi-latest",
		"ling-3.0-flash",
		"minimax-latest",
		"minimax-m3",
		"nemotron-3-nano-30b-a3b",
		"nemotron-3-ultra-550b-a55b",
		"riverflow-v2.5-fast",
		"riverflow-v2.5-pro",
		"step-3.7-flash",
		"text-max",
		"text-prime",
		"text-standard",
		"thesean-claude-haiku-4.5",
		"thesean-claude-opus-4.8",
		"thesean-claude-sonnet-5",
		"thesean-gpt-5.6-sol",
	}
	if !slices.Equal(active, want) {
		t.Fatalf("active New badges = %v, want %v", active, want)
	}
}
