package agentruntime

import (
	"context"
	"strings"

	"github.com/usehivy/hivy/internal/credentials"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/registry"
)

type modelRouteMetadata struct {
	CanonicalModelID string
	ProviderID       string
	UpstreamModelID  string
	ModelProfile     string
	ProviderOptions  map[string]any
	Capabilities     map[string]any
}

func resolveAgentModelRouteMetadata(ctx context.Context, deps CompileDeps, agent *model.Agent, modelID string) modelRouteMetadata {
	providerID := ""
	if deps.DB != nil && agent != nil && agent.OrgID != nil {
		if cred, err := credentials.ResolveForModel(ctx, deps.DB, nil, *agent.OrgID, modelID); err == nil && cred != nil {
			providerID = strings.TrimSpace(cred.ProviderID)
		}
	}
	return resolveModelRouteMetadata(modelID, providerID)
}

func resolveModelRouteMetadata(modelID, providerID string) modelRouteMetadata {
	modelID = strings.TrimSpace(modelID)
	meta := modelRouteMetadata{
		CanonicalModelID: modelID,
		UpstreamModelID:  modelID,
		ProviderOptions:  map[string]any{},
	}
	reg := registry.Global()
	if providerID == "" {
		providers := reg.ProviderPreferenceForModel(modelID)
		if len(providers) > 0 {
			providerID = providers[0]
		}
	}
	if providerID != "" {
		meta.ProviderID = providerID
	}
	if providerID != "" {
		if route, ok := reg.ResolveModel(providerID, modelID); ok {
			meta.CanonicalModelID = route.CanonicalID
			meta.ProviderID = route.ProviderID
			meta.UpstreamModelID = route.UpstreamID
			meta.Capabilities = map[string]any{
				"reasoning":         route.Model.Reasoning,
				"tool_call":         route.Model.ToolCall,
				"structured_output": route.Model.StructuredOutput,
			}
		}
	}
	meta.ModelProfile = detectModelProfile(meta.ProviderID, meta.CanonicalModelID, meta.UpstreamModelID, modelID)
	return meta
}

func detectModelProfile(providerID, canonicalModelID, upstreamModelID, modelID string) string {
	haystack := strings.ToLower(strings.Join([]string{
		providerID,
		canonicalModelID,
		upstreamModelID,
		modelID,
	}, " "))
	switch {
	case strings.Contains(haystack, "deepseek"):
		return "deepseek"
	case strings.Contains(haystack, "glm") ||
		strings.Contains(haystack, "z-ai") ||
		strings.Contains(haystack, "z_ai") ||
		strings.Contains(haystack, "zai") ||
		strings.Contains(haystack, "zhipu"):
		return "glm"
	case strings.Contains(haystack, "kimi") || strings.Contains(haystack, "moonshot"):
		return "kimi"
	case strings.Contains(haystack, "minimax"):
		return "minimax"
	case strings.Contains(haystack, "mimo"):
		return "mimo"
	case strings.Contains(haystack, "qwen"):
		return "qwen"
	default:
		return "openrouter_compatible"
	}
}
