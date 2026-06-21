package agentruntime

import (
	"encoding/json"
	"strings"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/providerheaders"
)

const (
	modelRequestHTTPReferer = providerheaders.OpenRouterHTTPReferer
	modelRequestAppTitle    = providerheaders.OpenRouterAppTitle
)

func proxyModel(cfg *config.Config, modelID string, route modelRouteMetadata) ModelConfig {
	return ModelConfig{
		Provider:         "openai_compatible",
		BaseURL:          cfg.ProxyOpenAIBaseURL(),
		ModelID:          modelID,
		CanonicalModelID: route.CanonicalModelID,
		ProviderID:       route.ProviderID,
		UpstreamModelID:  route.UpstreamModelID,
		ModelProfile:     route.ModelProfile,
		ProviderOptions:  route.ProviderOptions,
		Capabilities:     route.Capabilities,
		APIKeyEnv:        ProxyAPIKeyEnv,
		ExtraHeaders:     defaultModelRequestHeaders(),
	}
}

func ptrModel(m ModelConfig) *ModelConfig { return &m }

func defaultModelRequestHeaders() map[string]string {
	return map[string]string{
		"HTTP-Referer": modelRequestHTTPReferer,
		"X-Title":      modelRequestAppTitle,
	}
}

func ProxyModelConfig(cfg *config.Config, modelID, reasoningEffort string) ModelConfig {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		modelID = DefaultAgentModel
	}
	model := proxyModel(cfg, modelID, resolveModelRouteMetadata(modelID, ""))
	if effort := strings.TrimSpace(reasoningEffort); effort != "" {
		model.ReasoningEffort = &effort
	}
	return model
}

func defaultLimits() map[string]any {
	return map[string]any{
		"max_turns_per_session":     50,
		"input_token_budget":        180000,
		"output_token_budget":       8000,
		"tool_call_timeout_seconds": 60,
	}
}

// jsonArray decodes a JSONB array column into a []any. mcp_servers stores an
// array, so we unmarshal RawJSON directly rather than round-trip through
// map[string]any (which would produce {} and fail to decode as an array).
func jsonArray(raw model.RawJSON) []any {
	if len(raw) == 0 {
		return []any{}
	}
	var arr []any
	if err := json.Unmarshal([]byte(raw), &arr); err != nil {
		return []any{}
	}
	return arr
}
