package agentruntime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/model"
)

func TestControlPlaneOutboundChannels_EmitsAgentWebhookSpec(t *testing.T) {
	sandboxID := uuid.New()
	channels := ControlPlaneOutboundChannels(&config.Config{APIWebhookBaseURL: "https://api.hivy.test"}, sandboxID)
	if len(channels) != 1 {
		t.Fatalf("channels = %#v", channels)
	}
	channel, ok := channels[0].(map[string]any)
	if !ok {
		t.Fatalf("channel has wrong type: %#v", channels[0])
	}
	if channel["url"] != "https://api.hivy.test/internal/webhooks/agent/"+sandboxID.String() {
		t.Fatalf("url = %q", channel["url"])
	}
	if channel["secret_env"] != AgentEnvRuntimeSecret {
		t.Fatalf("secret env = %q", channel["secret_env"])
	}
}

func TestControlPlaneOutboundChannels_UsesAPIWebhookBaseURL(t *testing.T) {
	sandboxID := uuid.New()
	channels := ControlPlaneOutboundChannels(&config.Config{APIWebhookBaseURL: "http://host.docker.internal:8080"}, sandboxID)
	channel := channels[0].(map[string]any)
	if channel["url"] != "http://host.docker.internal:8080/internal/webhooks/agent/"+sandboxID.String() {
		t.Fatalf("url = %q", channel["url"])
	}
}

func TestCompile_ReferencesProxyEnvInsteadOfRawProviderKeys(t *testing.T) {
	orgID := uuid.New()
	agent := &model.Agent{ID: uuid.New(), OrgID: &orgID, Name: "Aria", Model: DefaultAgentModel}

	def, err := Compile(context.Background(), CompileDeps{Cfg: &config.Config{ProxyHost: "proxy.hivy.test"}}, agent)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if def.Model.APIKeyEnv != ProxyAPIKeyEnv {
		t.Fatalf("model.api_key_env = %q, want %q", def.Model.APIKeyEnv, ProxyAPIKeyEnv)
	}
	if def.Model.BaseURL != "https://proxy.hivy.test/v1" {
		t.Fatalf("model.base_url = %q", def.Model.BaseURL)
	}
	if def.MultimodalModel == nil || def.MultimodalModel.APIKeyEnv != ProxyAPIKeyEnv {
		t.Fatalf("multimodal_model.api_key_env = %#v, want %q", def.MultimodalModel, ProxyAPIKeyEnv)
	}
	if agentMCPAuthorizationHeader() != "Bearer ${"+ProxyAPIKeyEnv+"}" {
		t.Fatalf("MCP auth header references wrong env: %q", agentMCPAuthorizationHeader())
	}

	body, err := json.Marshal(def)
	if err != nil {
		t.Fatalf("marshal definition: %v", err)
	}
	if strings.Contains(string(body), `"tools"`) {
		t.Fatalf("runtime config should not define built-in tools: %s", string(body))
	}
	for _, forbidden := range AgentForbiddenRawProviderEnvKeys() {
		if strings.Contains(string(body), forbidden) {
			t.Fatalf("runtime config leaked raw provider key %s: %s", forbidden, string(body))
		}
	}
}

func TestCompile_UsesAgentModelWithDefaultFallback(t *testing.T) {
	orgID := uuid.New()
	agent := &model.Agent{ID: uuid.New(), OrgID: &orgID, Name: "Aria", Model: "deepseek-v4-pro"}

	def, err := Compile(context.Background(), CompileDeps{Cfg: &config.Config{}}, agent)
	if err != nil {
		t.Fatalf("compile custom model: %v", err)
	}
	if def.Model.ModelID != "deepseek-v4-pro" {
		t.Fatalf("model_id = %q, want deepseek-v4-pro", def.Model.ModelID)
	}

	agent.Model = " "
	def, err = Compile(context.Background(), CompileDeps{Cfg: &config.Config{}}, agent)
	if err != nil {
		t.Fatalf("compile blank model: %v", err)
	}
	if def.Model.ModelID != DefaultAgentModel {
		t.Fatalf("blank model fallback = %q, want %q", def.Model.ModelID, DefaultAgentModel)
	}
}

func TestCompile_IncludesCatalogSubAgents(t *testing.T) {
	orgID := uuid.New()
	rawSubAgents, err := json.Marshal(map[string]model.AgentCatalogSubAgent{
		"codebase-explorer": {
			Name:         "Codebase Explorer",
			Description:  "Maps code paths.",
			Model:        "qwen3.7-plus",
			Instructions: "Trace code paths with evidence.",
		},
	})
	if err != nil {
		t.Fatalf("marshal subagents: %v", err)
	}
	agent := &model.Agent{
		ID:    uuid.New(),
		OrgID: &orgID,
		Name:  "Hakaree",
		Model: "deepseek-v4-pro",
		AgentCatalog: &model.AgentCatalog{
			SubAgents: model.RawJSON(rawSubAgents),
		},
	}

	def, err := Compile(context.Background(), CompileDeps{Cfg: &config.Config{}}, agent)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	subAgent, ok := def.SubAgents["codebase-explorer"]
	if !ok {
		t.Fatalf("missing codebase-explorer subagent: %#v", def.SubAgents)
	}
	if subAgent.Agent.Name != "Codebase Explorer" {
		t.Fatalf("subagent name = %q", subAgent.Agent.Name)
	}
	if subAgent.Model.ModelID != "qwen3.7-plus" {
		t.Fatalf("subagent model = %q", subAgent.Model.ModelID)
	}
	cacheable := requireCacheableSegments(t, subAgent.SystemPrompt)
	if len(cacheable) < 2 {
		t.Fatalf("subagent cacheable segments = %d", len(cacheable))
	}
	instructionsContent := requirePromptString(t, requireStaticPromptSegment(t, cacheable[1]).Content)
	if !strings.Contains(instructionsContent, "<instructions>\nTrace code paths with evidence.\n</instructions>") {
		t.Fatalf("subagent instructions missing: %q", instructionsContent)
	}
	if len(def.Tools) != 0 {
		t.Fatalf("parent tools should be omitted for runtime defaults: %#v", def.Tools)
	}
	if len(subAgent.Tools) != 0 {
		t.Fatalf("subagent tools should be omitted for runtime defaults: %#v", subAgent.Tools)
	}
}
