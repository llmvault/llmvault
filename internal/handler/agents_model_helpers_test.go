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
