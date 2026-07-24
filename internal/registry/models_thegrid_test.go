package registry

import (
	"slices"
	"testing"
	"time"
)

func TestTheGridProviderContainsLiveInstrumentDirectory(t *testing.T) {
	provider, ok := Global().GetProvider("thegrid")
	if !ok {
		t.Fatal("The Grid provider not found")
	}
	if provider.API != "https://api.thegrid.ai/v1" {
		t.Fatalf("API = %q", provider.API)
	}

	wantIDs := []string{
		"agent-max",
		"agent-prime",
		"agent-standard",
		"bytedance-pro-latest",
		"claude-opus-latest",
		"code-max",
		"code-prime",
		"code-standard",
		"deepseek-pro-latest",
		"gemini-pro-latest",
		"glm-latest",
		"gpt-sol-latest",
		"kimi-latest",
		"minimax-latest",
		"text-max",
		"text-prime",
		"text-standard",
	}
	gotIDs := make([]string, 0, len(provider.Models))
	for id := range provider.Models {
		gotIDs = append(gotIDs, id)
	}
	slices.Sort(gotIDs)
	if !slices.Equal(gotIDs, wantIDs) {
		t.Fatalf("The Grid models = %v, want %v", gotIDs, wantIDs)
	}
}

func TestTheGridModelsExposeVerifiedMetadataAndNewWindow(t *testing.T) {
	tests := []struct {
		id            string
		family        string
		context       int64
		output        int64
		inputPrice    float64
		supportsImage bool
	}{
		{"text-standard", "text", 128000, 65536, 0.035, false},
		{"code-prime", "code", 196608, 131072, 0.120, false},
		{"agent-max", "agent", 1000000, 131072, 1.709, true},
		{"gpt-sol-latest", "lab", 1000000, 128000, 1.821, true},
		{"minimax-latest", "lab", 1000000, 512000, 0.118, false},
		{"kimi-latest", "lab", 262144, 131072, 1.000, true},
	}

	wantFrom := time.Date(2026, time.July, 24, 0, 0, 0, 0, time.UTC)
	wantTo := time.Date(2026, time.September, 24, 0, 0, 0, 0, time.UTC)
	for _, test := range tests {
		t.Run(test.id, func(t *testing.T) {
			resolved, ok := Global().ResolveModel("thegrid", test.id)
			if !ok {
				t.Fatalf("resolve %s", test.id)
			}
			model := resolved.Model
			if resolved.UpstreamID != test.id {
				t.Fatalf("upstream ID = %q", resolved.UpstreamID)
			}
			if model.Family != test.family || model.Limit == nil ||
				model.Limit.Context != test.context || model.Limit.Output != test.output {
				t.Fatalf("model metadata = %#v", model)
			}
			if model.Cost == nil || model.Cost.Input != test.inputPrice {
				t.Fatalf("model cost = %#v", model.Cost)
			}
			supportsImage := slices.Contains(model.Modalities.Input, "image")
			if supportsImage != test.supportsImage {
				t.Fatalf("image input = %v, want %v", supportsImage, test.supportsImage)
			}
			if model.NewFrom == nil || !model.NewFrom.Equal(wantFrom) ||
				model.NewTo == nil || !model.NewTo.Equal(wantTo) {
				t.Fatalf("new window = %v to %v", model.NewFrom, model.NewTo)
			}
		})
	}
}
