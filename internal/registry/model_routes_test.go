package registry

import "testing"

func TestResolveModel_ExplicitProviderRoutes(t *testing.T) {
	reg := Global()

	anthropic, ok := reg.ResolveModel("anthropic", "claude-sonnet-4.6")
	if !ok {
		t.Fatal("anthropic route not found")
	}
	if anthropic.UpstreamID != "claude-sonnet-4-6" {
		t.Fatalf("anthropic upstream = %q, want claude-sonnet-4-6", anthropic.UpstreamID)
	}
	if anthropic.Model.ID != "claude-sonnet-4.6" {
		t.Fatalf("canonical model id = %q", anthropic.Model.ID)
	}

	openrouter, ok := reg.ResolveModel("openrouter", "claude-sonnet-4.6")
	if !ok {
		t.Fatal("openrouter route not found")
	}
	if openrouter.UpstreamID != "anthropic/claude-sonnet-4.6" {
		t.Fatalf("openrouter upstream = %q, want anthropic/claude-sonnet-4.6", openrouter.UpstreamID)
	}
}

func TestSupportedHivyModelRoutesResolve(t *testing.T) {
	reg := Global()
	for _, hivyModel := range supportedHivyModels {
		for _, route := range hivyModel.Routes {
			if _, ok := reg.ResolveModel(route.ProviderID, hivyModel.ID); !ok {
				t.Fatalf("route %s via %s/%s did not resolve", hivyModel.ID, route.ProviderID, route.ModelID)
			}
		}
	}
}

func TestSupportedHivyModelIDsAreUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, hivyModel := range supportedHivyModels {
		if seen[hivyModel.ID] {
			t.Fatalf("duplicate canonical model ID %q", hivyModel.ID)
		}
		seen[hivyModel.ID] = true
	}
}

func TestResolveModel_ExplicitSameProviderRoute(t *testing.T) {
	route, ok := Global().ResolveModel("openai", "gpt-5.4")
	if !ok {
		t.Fatal("openai route not found")
	}
	if route.UpstreamID != "gpt-5.4" {
		t.Fatalf("upstream = %q, want gpt-5.4", route.UpstreamID)
	}
}

func TestResolveModel_GPT4OMiniRoutes(t *testing.T) {
	openai, ok := Global().ResolveModel("openai", "gpt-4o-mini")
	if !ok {
		t.Fatal("openai gpt-4o-mini route not found")
	}
	if openai.UpstreamID != "gpt-4o-mini" {
		t.Fatalf("openai upstream = %q, want gpt-4o-mini", openai.UpstreamID)
	}

	openrouter, ok := Global().ResolveModel("openrouter", "gpt-4o-mini")
	if !ok {
		t.Fatal("openrouter gpt-4o-mini route not found")
	}
	if openrouter.UpstreamID != "openai/gpt-4o-mini" {
		t.Fatalf("openrouter upstream = %q, want openai/gpt-4o-mini", openrouter.UpstreamID)
	}
}

func TestElevenLabsScribeV2Catalog(t *testing.T) {
	reg := Global()

	provider, ok := reg.GetProvider("elevenlabs")
	if !ok {
		t.Fatal("elevenlabs provider not found")
	}
	if provider.API != "https://api.elevenlabs.io" {
		t.Fatalf("elevenlabs API = %q, want https://api.elevenlabs.io", provider.API)
	}

	route, ok := reg.ResolveModel("elevenlabs", "scribe-v2")
	if !ok {
		t.Fatal("scribe-v2 route not found")
	}
	if route.UpstreamID != "scribe_v2" {
		t.Fatalf("scribe-v2 upstream = %q, want scribe_v2", route.UpstreamID)
	}
	if route.Model.Family != "scribe" {
		t.Fatalf("scribe-v2 family = %q, want scribe", route.Model.Family)
	}
	if route.Model.Modalities == nil {
		t.Fatal("scribe-v2 modalities missing")
	}
	if len(route.Model.Modalities.Input) != 1 || route.Model.Modalities.Input[0] != "audio" {
		t.Fatalf("scribe-v2 input modalities = %v, want [audio]", route.Model.Modalities.Input)
	}
	if len(route.Model.Modalities.Output) != 1 || route.Model.Modalities.Output[0] != "text" {
		t.Fatalf("scribe-v2 output modalities = %v, want [text]", route.Model.Modalities.Output)
	}
}

func TestOpenRouterQwen37Catalog(t *testing.T) {
	reg := Global()

	maxRoute, ok := reg.ResolveModel("openrouter", "qwen3.7-max")
	if !ok {
		t.Fatal("qwen3.7-max route not found")
	}
	if maxRoute.UpstreamID != "qwen/qwen3.7-max" {
		t.Fatalf("qwen3.7-max upstream = %q, want qwen/qwen3.7-max", maxRoute.UpstreamID)
	}
	if maxRoute.Model.Cost == nil || maxRoute.Model.Cost.Input != 1.25 || maxRoute.Model.Cost.Output != 3.75 {
		t.Fatalf("qwen3.7-max cost = %#v", maxRoute.Model.Cost)
	}
	if maxRoute.Model.Limit == nil || maxRoute.Model.Limit.Context != 1000000 || maxRoute.Model.Limit.Output != 65536 {
		t.Fatalf("qwen3.7-max limit = %#v", maxRoute.Model.Limit)
	}

	plusRoute, ok := reg.ResolveModel("openrouter", "qwen3.7-plus")
	if !ok {
		t.Fatal("qwen3.7-plus route not found")
	}
	if plusRoute.UpstreamID != "qwen/qwen3.7-plus" {
		t.Fatalf("qwen3.7-plus upstream = %q, want qwen/qwen3.7-plus", plusRoute.UpstreamID)
	}
	if plusRoute.Model.Cost == nil || plusRoute.Model.Cost.Input != 0.4 || plusRoute.Model.Cost.Output != 1.6 {
		t.Fatalf("qwen3.7-plus cost = %#v", plusRoute.Model.Cost)
	}
	if plusRoute.Model.Limit == nil || plusRoute.Model.Limit.Context != 1000000 || plusRoute.Model.Limit.Output != 65536 {
		t.Fatalf("qwen3.7-plus limit = %#v", plusRoute.Model.Limit)
	}
	if !plusRoute.Model.ToolCall || !plusRoute.Model.StructuredOutput {
		t.Fatalf("qwen3.7-plus capabilities = tool_call:%v structured:%v", plusRoute.Model.ToolCall, plusRoute.Model.StructuredOutput)
	}
}

func TestOpenRouterGrokAndNemotronCatalog(t *testing.T) {
	reg := Global()

	grok, ok := reg.ResolveModel("openrouter", "grok-4.3")
	if !ok {
		t.Fatal("grok-4.3 route not found")
	}
	if grok.UpstreamID != "x-ai/grok-4.3" {
		t.Fatalf("grok-4.3 upstream = %q, want x-ai/grok-4.3", grok.UpstreamID)
	}
	if grok.Model.Cost == nil || grok.Model.Cost.Input != 1.25 || grok.Model.Cost.Output != 2.5 || grok.Model.Cost.CacheRead != 0.2 {
		t.Fatalf("grok-4.3 cost = %#v", grok.Model.Cost)
	}
	if grok.Model.Limit == nil || grok.Model.Limit.Context != 1000000 || grok.Model.Limit.Output != 1000000 {
		t.Fatalf("grok-4.3 limit = %#v", grok.Model.Limit)
	}
	if !grok.Model.ToolCall || !grok.Model.StructuredOutput || !grok.Model.Reasoning {
		t.Fatalf("grok-4.3 capabilities = tool_call:%v structured:%v reasoning:%v", grok.Model.ToolCall, grok.Model.StructuredOutput, grok.Model.Reasoning)
	}

	nemotron, ok := reg.ResolveModel("openrouter", "nemotron-3-ultra-550b-a55b")
	if !ok {
		t.Fatal("nemotron-3-ultra-550b-a55b route not found")
	}
	if nemotron.UpstreamID != "nvidia/nemotron-3-ultra-550b-a55b" {
		t.Fatalf("nemotron upstream = %q, want nvidia/nemotron-3-ultra-550b-a55b", nemotron.UpstreamID)
	}
	if nemotron.Model.Cost == nil || nemotron.Model.Cost.Input != 0.5 || nemotron.Model.Cost.Output != 2.5 || nemotron.Model.Cost.CacheRead != 0.15 {
		t.Fatalf("nemotron cost = %#v", nemotron.Model.Cost)
	}
	if nemotron.Model.Limit == nil || nemotron.Model.Limit.Context != 262144 || nemotron.Model.Limit.Output != 16384 {
		t.Fatalf("nemotron limit = %#v", nemotron.Model.Limit)
	}
	if !nemotron.Model.ToolCall || !nemotron.Model.StructuredOutput || !nemotron.Model.OpenWeights {
		t.Fatalf("nemotron capabilities = tool_call:%v structured:%v open_weights:%v", nemotron.Model.ToolCall, nemotron.Model.StructuredOutput, nemotron.Model.OpenWeights)
	}
}

func TestOpenRouterGemini31FlashLiteCatalog(t *testing.T) {
	route, ok := Global().ResolveModel("openrouter", "gemini-3.1-flash-lite")
	if !ok {
		t.Fatal("gemini-3.1-flash-lite route not found")
	}
	if route.UpstreamID != "google/gemini-3.1-flash-lite" {
		t.Fatalf("gemini-3.1-flash-lite upstream = %q, want google/gemini-3.1-flash-lite", route.UpstreamID)
	}
	if route.Model.Cost == nil || route.Model.Cost.Input != 0.25 || route.Model.Cost.Output != 1.5 || route.Model.Cost.CacheRead != 0.025 || route.Model.Cost.CacheWrite != 0.083333 {
		t.Fatalf("gemini-3.1-flash-lite cost = %#v", route.Model.Cost)
	}
	if route.Model.Limit == nil || route.Model.Limit.Context != 1048576 || route.Model.Limit.Output != 64000 {
		t.Fatalf("gemini-3.1-flash-lite limit = %#v", route.Model.Limit)
	}
	if route.Model.Knowledge != "2025-01" {
		t.Fatalf("gemini-3.1-flash-lite knowledge = %q", route.Model.Knowledge)
	}
	if !route.Model.ToolCall || !route.Model.StructuredOutput || !route.Model.Reasoning {
		t.Fatalf("gemini-3.1-flash-lite capabilities = tool_call:%v structured:%v reasoning:%v", route.Model.ToolCall, route.Model.StructuredOutput, route.Model.Reasoning)
	}
}

func TestResolveModel_RejectsProviderModelNotInHivyCatalog(t *testing.T) {
	if _, ok := Global().ResolveModel("openai", "gpt-5-thinking"); ok {
		t.Fatal("provider model resolved without explicit Hivy catalog entry")
	}
}

func TestCanonicalModelsForProviders_DeduplicatesExplicitRoutes(t *testing.T) {
	models := Global().CanonicalModelsForProviders([]string{"anthropic", "openrouter"})
	count := 0
	var providers []string
	upstreamCount := 0
	for _, model := range models {
		switch model.ID {
		case "claude-sonnet-4.6":
			count++
			providers = model.ProviderIDs
		case "anthropic/claude-sonnet-4.6":
			upstreamCount++
		}
	}
	if count != 1 {
		t.Fatalf("claude-sonnet-4.6 count = %d, want 1", count)
	}
	if upstreamCount != 0 {
		t.Fatalf("anthropic/claude-sonnet-4.6 upstream count = %d, want 0", upstreamCount)
	}
	if len(providers) != 2 || providers[0] != "anthropic" || providers[1] != "openrouter" {
		t.Fatalf("providers = %v, want [anthropic openrouter]", providers)
	}
}

func TestValidateCanonicalModelRejectsExplicitUpstreamAlias(t *testing.T) {
	err := Global().ValidateCanonicalModel("anthropic/claude-sonnet-4.6")
	if err == nil {
		t.Fatal("expected upstream alias to be rejected as a canonical model")
	}
}
