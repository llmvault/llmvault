package tasks

import (
	"testing"

	"github.com/usehivy/hivy/internal/registry"
)

func TestResolveReflectionModelDefaultsToAtlasCloudGPT54Mini(t *testing.T) {
	t.Setenv(reflectionProviderEnv, "")
	t.Setenv(reflectionModelEnv, "")
	t.Setenv(reflectionTemperatureEnv, "")

	providerID, modelID, temperature := ResolveReflectionModel(registry.Global())
	if providerID != "atlascloud" {
		t.Fatalf("provider=%q want atlascloud", providerID)
	}
	if modelID != "openai/gpt-5.4-mini" {
		t.Fatalf("model=%q want openai/gpt-5.4-mini", modelID)
	}
	if temperature != reflectionDefaultTemperature {
		t.Fatalf("temperature=%v want %v", temperature, reflectionDefaultTemperature)
	}
}
