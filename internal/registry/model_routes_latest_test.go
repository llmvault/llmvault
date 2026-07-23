package registry

import "testing"

func TestAtlasCloudGPT56Catalog(t *testing.T) {
	for _, test := range []struct {
		canonicalID string
		upstreamID  string
	}{
		{"gpt-5.6-luna", "openai/gpt-5.6-luna"},
		{"gpt-5.6-terra", "openai/gpt-5.6-terra"},
		{"gpt-5.6-sol", "openai/gpt-5.6-sol"},
	} {
		t.Run(test.canonicalID, func(t *testing.T) {
			route, ok := Global().ResolveModel("atlascloud", test.canonicalID)
			if !ok {
				t.Fatalf("%s route not found", test.canonicalID)
			}
			if route.UpstreamID != test.upstreamID {
				t.Fatalf("upstream = %q, want %q", route.UpstreamID, test.upstreamID)
			}
		})
	}
}

func TestNovitaMiniMaxM3AndGLM52Catalog(t *testing.T) {
	reg := Global()

	m3, ok := reg.ResolveModel("novita", "minimax-m3")
	if !ok {
		t.Fatal("minimax-m3 route not found")
	}
	if m3.UpstreamID != "minimax/minimax-m3" {
		t.Fatalf("minimax-m3 upstream = %q, want minimax/minimax-m3", m3.UpstreamID)
	}
	if m3.Model.Cost == nil || m3.Model.Cost.Input != 0.3 || m3.Model.Cost.Output != 1.2 || m3.Model.Cost.CacheRead != 0.06 {
		t.Fatalf("minimax-m3 cost = %#v", m3.Model.Cost)
	}

	glm, ok := reg.ResolveModel("novita", "glm-5.2")
	if !ok {
		t.Fatal("glm-5.2 route not found")
	}
	if glm.UpstreamID != "zai-org/glm-5.2" {
		t.Fatalf("glm-5.2 upstream = %q, want zai-org/glm-5.2", glm.UpstreamID)
	}
	if glm.Model.Cost == nil || glm.Model.Cost.Input != 1.4 || glm.Model.Cost.Output != 4.4 || glm.Model.Cost.CacheRead != 0.26 {
		t.Fatalf("glm-5.2 cost = %#v", glm.Model.Cost)
	}
}

func TestNovitaTencentHy3Catalog(t *testing.T) {
	hy3, ok := Global().ResolveModel("novita", "hy3")
	if !ok {
		t.Fatal("hy3 route not found")
	}
	if hy3.UpstreamID != "tencent/hy3" {
		t.Fatalf("hy3 upstream = %q, want tencent/hy3", hy3.UpstreamID)
	}
	if hy3.Model.Cost == nil || hy3.Model.Cost.Input != 0.14 || hy3.Model.Cost.Output != 0.58 || hy3.Model.Cost.CacheRead != 0.035 {
		t.Fatalf("hy3 cost = %#v", hy3.Model.Cost)
	}
}

func TestAtlasCloudGrok45Catalog(t *testing.T) {
	grok, ok := Global().ResolveModel("atlascloud", "grok-4.5")
	if !ok {
		t.Fatal("grok-4.5 route not found")
	}
	if grok.UpstreamID != "xai/grok-4.5" {
		t.Fatalf("grok-4.5 upstream = %q, want xai/grok-4.5", grok.UpstreamID)
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

	if _, ok := reg.ResolveModel("openrouter", "kimi-k2.7-code"); ok {
		t.Fatal("kimi-k2.7-code unexpectedly resolves through OpenRouter")
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
