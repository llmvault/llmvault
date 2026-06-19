package handler

import (
	"testing"

	"github.com/usehivy/hivy/internal/model"
)

func TestNormalizeAgentAvailableModelsDefaultsToAgentModel(t *testing.T) {
	got := normalizeAgentAvailableModels("gpt-4o-mini", nil)
	if len(got) != 1 || got[0] != "gpt-4o-mini" {
		t.Fatalf("available models = %#v", got)
	}
}

func TestNormalizeAgentAvailableModelsTrimsAndDeduplicates(t *testing.T) {
	input := []string{" gpt-4o-mini ", "", "gpt-4o-mini", "claude-sonnet-4.6"}
	got := normalizeAgentAvailableModels("gpt-4o-mini", &input)
	want := []string{"gpt-4o-mini", "claude-sonnet-4.6"}
	if len(got) != len(want) {
		t.Fatalf("available models = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("available models = %#v, want %#v", got, want)
		}
	}
}

func TestAgentAllowsModelFallsBackToDefaultModel(t *testing.T) {
	agent := &model.Agent{Model: "gpt-4o-mini"}
	if !agentAllowsModel(agent, "gpt-4o-mini") {
		t.Fatal("expected default model to be allowed when available_models is empty")
	}
	if agentAllowsModel(agent, "claude-sonnet-4.6") {
		t.Fatal("unexpectedly allowed non-default model")
	}
}

func TestAgentAllowsModelUsesCatalogAvailableModels(t *testing.T) {
	agent := &model.Agent{
		Model:           "deepseek-v4-flash",
		AvailableModels: []string{"deepseek-v4-flash"},
		AgentCatalog: &model.AgentCatalog{
			Model:           "deepseek-v4-pro",
			AvailableModels: []string{"deepseek-v4-pro", "qwen3.7-plus"},
		},
	}
	if !agentAllowsModel(agent, "qwen3.7-plus") {
		t.Fatal("expected catalog available model to be allowed")
	}
	if agentAllowsModel(agent, "claude-sonnet-4.6") {
		t.Fatal("unexpectedly allowed model outside catalog")
	}
}

func TestAgentAllowsModelMergesCatalogAndInstalledAvailableModels(t *testing.T) {
	agent := &model.Agent{
		Model:           "deepseek-v4-flash",
		AvailableModels: []string{"deepseek-v4-flash"},
		AgentCatalog: &model.AgentCatalog{
			Model:           "deepseek-v4-pro",
			AvailableModels: []string{"deepseek-v4-pro", "qwen3.7-plus"},
		},
	}
	if !agentAllowsModel(agent, "deepseek-v4-flash") {
		t.Fatal("expected installed available model to be allowed")
	}
	if !agentAllowsModel(agent, "qwen3.7-plus") {
		t.Fatal("expected catalog available model to be allowed")
	}
}
