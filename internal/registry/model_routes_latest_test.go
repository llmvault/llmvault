package registry

import "testing"

func TestOpenRouterMiniMaxM3AndGLM52Catalog(t *testing.T) {
	reg := Global()

	m3, ok := reg.ResolveModel("openrouter", "minimax-m3")
	if !ok {
		t.Fatal("minimax-m3 route not found")
	}
	if m3.UpstreamID != "minimax/minimax-m3" {
		t.Fatalf("minimax-m3 upstream = %q, want minimax/minimax-m3", m3.UpstreamID)
	}
	if m3.Model.Cost == nil || m3.Model.Cost.Input != 0.3 || m3.Model.Cost.Output != 1.2 || m3.Model.Cost.CacheRead != 0.06 {
		t.Fatalf("minimax-m3 cost = %#v", m3.Model.Cost)
	}
	if m3.Model.Limit == nil || m3.Model.Limit.Context != 524288 || m3.Model.Limit.Output != 512000 {
		t.Fatalf("minimax-m3 limit = %#v", m3.Model.Limit)
	}
	if !m3.Model.ToolCall || !m3.Model.StructuredOutput || !m3.Model.Reasoning || !m3.Model.OpenWeights {
		t.Fatalf("minimax-m3 capabilities = tool_call:%v structured:%v reasoning:%v open_weights:%v", m3.Model.ToolCall, m3.Model.StructuredOutput, m3.Model.Reasoning, m3.Model.OpenWeights)
	}

	glm, ok := reg.ResolveModel("openrouter", "glm-5.2")
	if !ok {
		t.Fatal("glm-5.2 route not found")
	}
	if glm.UpstreamID != "z-ai/glm-5.2" {
		t.Fatalf("glm-5.2 upstream = %q, want z-ai/glm-5.2", glm.UpstreamID)
	}
	if glm.Model.Cost == nil || glm.Model.Cost.Input != 1.4 || glm.Model.Cost.Output != 4.4 || glm.Model.Cost.CacheRead != 0.26 {
		t.Fatalf("glm-5.2 cost = %#v", glm.Model.Cost)
	}
	if glm.Model.Limit == nil || glm.Model.Limit.Context != 262144 || glm.Model.Limit.Output != 262144 {
		t.Fatalf("glm-5.2 limit = %#v", glm.Model.Limit)
	}
	if !glm.Model.ToolCall || !glm.Model.StructuredOutput || !glm.Model.Reasoning || glm.Model.OpenWeights {
		t.Fatalf("glm-5.2 capabilities = tool_call:%v structured:%v reasoning:%v open_weights:%v", glm.Model.ToolCall, glm.Model.StructuredOutput, glm.Model.Reasoning, glm.Model.OpenWeights)
	}
}

func TestOpenRouterTencentHy3Catalog(t *testing.T) {
	hy3, ok := Global().ResolveModel("openrouter", "hy3")
	if !ok {
		t.Fatal("hy3 route not found")
	}
	if hy3.UpstreamID != "tencent/hy3" {
		t.Fatalf("hy3 upstream = %q, want tencent/hy3", hy3.UpstreamID)
	}
	if hy3.Model.Cost == nil || hy3.Model.Cost.Input != 0.2 || hy3.Model.Cost.Output != 0.8 || hy3.Model.Cost.CacheRead != 0.5 {
		t.Fatalf("hy3 cost = %#v", hy3.Model.Cost)
	}
	if hy3.Model.Limit == nil || hy3.Model.Limit.Context != 202752 || hy3.Model.Limit.Output != 131072 {
		t.Fatalf("hy3 limit = %#v", hy3.Model.Limit)
	}
	if !hy3.Model.ToolCall || !hy3.Model.StructuredOutput || !hy3.Model.Reasoning || !hy3.Model.OpenWeights {
		t.Fatalf("hy3 capabilities = tool_call:%v structured:%v reasoning:%v open_weights:%v", hy3.Model.ToolCall, hy3.Model.StructuredOutput, hy3.Model.Reasoning, hy3.Model.OpenWeights)
	}
}

func TestOpenRouterGrok45Catalog(t *testing.T) {
	grok, ok := Global().ResolveModel("openrouter", "grok-4.5")
	if !ok {
		t.Fatal("grok-4.5 route not found")
	}
	if grok.UpstreamID != "x-ai/grok-4.5" {
		t.Fatalf("grok-4.5 upstream = %q, want x-ai/grok-4.5", grok.UpstreamID)
	}
	if grok.Model.Cost == nil || grok.Model.Cost.Input != 2 || grok.Model.Cost.Output != 6 || grok.Model.Cost.CacheRead != 0.5 {
		t.Fatalf("grok-4.5 cost = %#v", grok.Model.Cost)
	}
	if grok.Model.Limit == nil || grok.Model.Limit.Context != 500000 || grok.Model.Limit.Output != 500000 {
		t.Fatalf("grok-4.5 limit = %#v", grok.Model.Limit)
	}
	if !grok.Model.ToolCall || !grok.Model.StructuredOutput || !grok.Model.Reasoning || grok.Model.OpenWeights {
		t.Fatalf("grok-4.5 capabilities = tool_call:%v structured:%v reasoning:%v open_weights:%v", grok.Model.ToolCall, grok.Model.StructuredOutput, grok.Model.Reasoning, grok.Model.OpenWeights)
	}
}

func TestOpenRouterLagunaM1Catalog(t *testing.T) {
	laguna, ok := Global().ResolveModel("openrouter", "laguna-m.1")
	if !ok {
		t.Fatal("laguna-m.1 route not found")
	}
	if laguna.UpstreamID != "poolside/laguna-m.1" {
		t.Fatalf("laguna-m.1 upstream = %q, want poolside/laguna-m.1", laguna.UpstreamID)
	}
	if laguna.Model.Cost == nil || laguna.Model.Cost.Input != 0.2 || laguna.Model.Cost.Output != 0.4 || laguna.Model.Cost.CacheRead != 0.1 {
		t.Fatalf("laguna-m.1 cost = %#v", laguna.Model.Cost)
	}
	if laguna.Model.Limit == nil || laguna.Model.Limit.Context != 262144 || laguna.Model.Limit.Output != 32768 {
		t.Fatalf("laguna-m.1 limit = %#v", laguna.Model.Limit)
	}
	if !laguna.Model.ToolCall || laguna.Model.StructuredOutput || !laguna.Model.Reasoning || !laguna.Model.OpenWeights {
		t.Fatalf("laguna-m.1 capabilities = tool_call:%v structured:%v reasoning:%v open_weights:%v", laguna.Model.ToolCall, laguna.Model.StructuredOutput, laguna.Model.Reasoning, laguna.Model.OpenWeights)
	}
}

func TestKimiK27CodeCatalog(t *testing.T) {
	reg := Global()

	moonshot, ok := reg.ResolveModel("moonshotai", "kimi-k2.7-code")
	if !ok {
		t.Fatal("moonshot kimi-k2.7-code route not found")
	}
	if moonshot.UpstreamID != "kimi-k2.7-code" {
		t.Fatalf("moonshot upstream = %q, want kimi-k2.7-code", moonshot.UpstreamID)
	}
	if moonshot.Model.Cost == nil || moonshot.Model.Cost.Input != 0.95 || moonshot.Model.Cost.Output != 4 || moonshot.Model.Cost.CacheRead != 0.19 {
		t.Fatalf("moonshot kimi-k2.7-code cost = %#v", moonshot.Model.Cost)
	}
	if moonshot.Model.Limit == nil || moonshot.Model.Limit.Context != 262144 || moonshot.Model.Limit.Output != 262144 {
		t.Fatalf("moonshot kimi-k2.7-code limit = %#v", moonshot.Model.Limit)
	}
	if !moonshot.Model.ToolCall || !moonshot.Model.StructuredOutput || !moonshot.Model.Reasoning || !moonshot.Model.OpenWeights {
		t.Fatalf("moonshot kimi-k2.7-code capabilities = tool_call:%v structured:%v reasoning:%v open_weights:%v", moonshot.Model.ToolCall, moonshot.Model.StructuredOutput, moonshot.Model.Reasoning, moonshot.Model.OpenWeights)
	}

	openrouter, ok := reg.ResolveModel("openrouter", "kimi-k2.7-code")
	if !ok {
		t.Fatal("openrouter kimi-k2.7-code route not found")
	}
	if openrouter.UpstreamID != "moonshotai/kimi-k2.7-code" {
		t.Fatalf("openrouter upstream = %q, want moonshotai/kimi-k2.7-code", openrouter.UpstreamID)
	}
	if openrouter.Model.Cost == nil || openrouter.Model.Cost.Input != 0.74 || openrouter.Model.Cost.Output != 3.5 || openrouter.Model.Cost.CacheRead != 0.15 {
		t.Fatalf("openrouter kimi-k2.7-code cost = %#v", openrouter.Model.Cost)
	}
	if openrouter.Model.Limit == nil || openrouter.Model.Limit.Context != 262144 || openrouter.Model.Limit.Output != 16384 {
		t.Fatalf("openrouter kimi-k2.7-code limit = %#v", openrouter.Model.Limit)
	}
	if !openrouter.Model.ToolCall || !openrouter.Model.StructuredOutput || !openrouter.Model.Reasoning || !openrouter.Model.OpenWeights {
		t.Fatalf("openrouter kimi-k2.7-code capabilities = tool_call:%v structured:%v reasoning:%v open_weights:%v", openrouter.Model.ToolCall, openrouter.Model.StructuredOutput, openrouter.Model.Reasoning, openrouter.Model.OpenWeights)
	}

	fireworks, ok := reg.GetProvider("fireworks-ai")
	if !ok {
		t.Fatal("fireworks-ai provider not found")
	}
	fireworksModel, ok := fireworks.Models["accounts/fireworks/models/kimi-k2p7-code"]
	if !ok {
		t.Fatal("fireworks kimi-k2p7-code model not found")
	}
	if fireworksModel.Cost == nil || fireworksModel.Cost.Input != 0.95 || fireworksModel.Cost.Output != 4 || fireworksModel.Cost.CacheRead != 0.19 {
		t.Fatalf("fireworks kimi-k2p7-code cost = %#v", fireworksModel.Cost)
	}
}
