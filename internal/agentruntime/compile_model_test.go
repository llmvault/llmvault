package agentruntime

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/model"
)

func TestControlPlaneOutboundChannels_EmitsRuntimeWebSocketSpec(t *testing.T) {
	sandboxID := uuid.New()
	channels := ControlPlaneOutboundChannels(&config.Config{APIWebhookBaseURL: "https://api.hivy.test"}, sandboxID)
	if len(channels) != 2 {
		t.Fatalf("channels = %#v", channels)
	}
	streamChannel := outboundChannelByName(t, channels, "control-plane-runtime-stream")
	if streamChannel["type"] != "websocket" {
		t.Fatalf("stream type = %q", streamChannel["type"])
	}
	if streamChannel["url"] != "wss://api.hivy.test/internal/runtime-events/sandboxes/"+sandboxID.String()+"/sessions/{session_id}/ws" {
		t.Fatalf("stream url = %q", streamChannel["url"])
	}
	if streamChannel["secret_env"] != AgentEnvRuntimeSecret {
		t.Fatalf("stream secret env = %q", streamChannel["secret_env"])
	}

	stateChannel := outboundChannelByName(t, channels, "control-plane-turn-state")
	if stateChannel["type"] != "webhook" {
		t.Fatalf("turn-state type = %q", stateChannel["type"])
	}
	if stateChannel["url"] != "https://api.hivy.test/internal/runtime-events/sandboxes/"+sandboxID.String()+"/turn-state" {
		t.Fatalf("turn-state url = %q", stateChannel["url"])
	}
	if stateChannel["secret_env"] != AgentEnvRuntimeSecret {
		t.Fatalf("turn-state secret env = %q", stateChannel["secret_env"])
	}
}

func TestControlPlaneOutboundChannels_UsesAPIWebhookBaseURL(t *testing.T) {
	sandboxID := uuid.New()
	channels := ControlPlaneOutboundChannels(&config.Config{APIWebhookBaseURL: "http://host.docker.internal:8080"}, sandboxID)
	channel := outboundChannelByName(t, channels, "control-plane-runtime-stream")
	if channel["url"] != "ws://host.docker.internal:8080/internal/runtime-events/sandboxes/"+sandboxID.String()+"/sessions/{session_id}/ws" {
		t.Fatalf("url = %q", channel["url"])
	}
	stateChannel := outboundChannelByName(t, channels, "control-plane-turn-state")
	if stateChannel["url"] != "http://host.docker.internal:8080/internal/runtime-events/sandboxes/"+sandboxID.String()+"/turn-state" {
		t.Fatalf("turn-state url = %q", stateChannel["url"])
	}
}

func outboundChannelByName(t *testing.T, channels []any, name string) map[string]any {
	t.Helper()
	for _, raw := range channels {
		channel, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("channel has wrong type: %#v", raw)
		}
		if channel["name"] == name {
			return channel
		}
	}
	t.Fatalf("missing outbound channel %q in %#v", name, channels)
	return nil
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
	if agentMCPAuthorizationHeader() != "Bearer ${"+ProxyAPIKeyEnv+"}" {
		t.Fatalf("MCP auth header references wrong env: %q", agentMCPAuthorizationHeader())
	}

	body, err := json.Marshal(def)
	if err != nil {
		t.Fatalf("marshal definition: %v", err)
	}
	if !strings.Contains(string(body), `"type":"builtin.drive_upload"`) || !strings.Contains(string(body), `"type":"builtin.drive_download"`) {
		t.Fatalf("runtime config should include locked drive tools: %s", string(body))
	}
	if def.Model.CanonicalModelID != DefaultAgentModel ||
		def.Model.ProviderID == "" ||
		def.Model.UpstreamModelID == "" ||
		def.Model.ModelProfile != "deepseek" {
		t.Fatalf("model route metadata = %#v", def.Model)
	}
	if def.Model.ReasoningEffort != nil || def.Model.Temperature != nil || def.Model.MaxOutputTokens != nil {
		t.Fatalf("model options should be sparse by default: %#v", def.Model)
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

func TestCompile_EmitsConfiguredRuntimeToolsAndIgnoresRetiredTools(t *testing.T) {
	orgID := uuid.New()
	agent := &model.Agent{
		ID:    uuid.New(),
		OrgID: &orgID,
		Name:  "Hakaree",
		Model: DefaultAgentModel,
		Tools: model.JSON{
			"bash":              map[string]any{"timeout_seconds": 25},
			"read_file":         true,
			"file_search":       true,
			"glob":              true,
			"grep":              map[string]any{"max_results": 25},
			"multi_grep":        true,
			"check_bash_status": true,
			"lsp":               false,
			"web_fetch":         true,
		},
	}

	def, err := Compile(context.Background(), CompileDeps{Cfg: &config.Config{}}, agent)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if got, want := runtimeToolTypes(def.Tools), []string{"builtin.bash", "builtin.read_file", "builtin.drive_upload", "builtin.drive_download"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("runtime tool types = %#v, want %#v", got, want)
	}
	bash := def.Tools[0]["config"].(map[string]any)
	if bash["timeout_seconds"] != 25 {
		t.Fatalf("bash timeout_seconds = %#v", bash["timeout_seconds"])
	}
}

func TestCompile_EmitsCatalogAndSubAgentRuntimeTools(t *testing.T) {
	orgID := uuid.New()
	rawSubAgents, err := json.Marshal(map[string]model.AgentCatalogSubAgent{
		"codebase-explorer": {
			Name:         "Codebase Explorer",
			Description:  "Maps code paths.",
			Model:        "qwen3.7-plus",
			Tools:        model.JSON{"bash": true, "read_file": true, "lsp": true},
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
			Tools:     model.JSON{"bash": true, "read_file": true},
			SubAgents: model.RawJSON(rawSubAgents),
		},
	}

	def, err := Compile(context.Background(), CompileDeps{Cfg: &config.Config{}}, agent)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if got, want := runtimeToolTypes(def.Tools), []string{"builtin.bash", "builtin.read_file", "builtin.drive_upload", "builtin.drive_download"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("parent runtime tools = %#v, want %#v", got, want)
	}
	subAgent := def.SubAgents["codebase-explorer"]
	if subAgent == nil {
		t.Fatalf("missing codebase-explorer subagent: %#v", def.SubAgents)
	}
	if got, want := runtimeToolTypes(subAgent.Tools), []string{"builtin.bash", "builtin.read_file", "builtin.lsp"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("subagent runtime tools = %#v, want %#v", got, want)
	}
}

func TestCompile_LoadsCatalogRuntimeToolsByID(t *testing.T) {
	db := connectCompileTestDB(t)
	org := model.Org{Name: "Tool Catalog-" + uuid.NewString()}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	catalog := model.AgentCatalog{
		ID:        uuid.New(),
		Slug:      "runtime-tools-" + uuid.NewString(),
		Name:      "Runtime Tools",
		Tools:     model.JSON{"bash": true, "read_file": true},
		SubAgents: model.RawJSON("{}"),
		Manifest:  model.RawJSON("{}"),
		Status:    model.AgentCatalogStatusActive,
	}
	if err := db.Create(&catalog).Error; err != nil {
		t.Fatalf("create catalog: %v", err)
	}
	team := seedCompileTeam(t, db, org.ID)
	agent := model.Agent{
		ID:             uuid.New(),
		OrgID:          &org.ID,
		TeamID:         team.ID,
		AgentCatalogID: &catalog.ID,
		Name:           "Hakaree",
		Model:          "deepseek-v4-pro",
		Tools:          model.JSON{},
		McpServers:     model.RawJSON("[]"),
		Skills:         model.JSON{},
		RuntimeConfig:  model.JSON{},
		Permissions:    model.JSON{},
		Resources:      model.JSON{},
		Status:         "active",
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	t.Cleanup(func() {
		db.Where("id = ?", agent.ID).Delete(&model.Agent{})
		db.Where("id = ?", catalog.ID).Delete(&model.AgentCatalog{})
		db.Where("id = ?", org.ID).Delete(&model.Org{})
	})

	def, err := Compile(context.Background(), CompileDeps{DB: db, Cfg: &config.Config{}}, &agent)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if got, want := runtimeToolTypes(def.Tools), []string{"builtin.bash", "builtin.read_file", "builtin.drive_upload", "builtin.drive_download"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("runtime tools = %#v, want %#v", got, want)
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
	instructionsContent := requirePromptString(t, requireStaticPromptSegmentByTitle(t, cacheable, "Instructions").Content)
	if !strings.Contains(instructionsContent, "<instructions>\nTrace code paths with evidence.\n</instructions>") {
		t.Fatalf("subagent instructions missing: %q", instructionsContent)
	}
	if got, want := runtimeToolTypes(def.Tools), []string{"builtin.drive_upload", "builtin.drive_download"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("parent locked drive tools = %#v, want %#v", got, want)
	}
	// A sub-agent with no configured tools defaults to read_file instead of
	// compiling tool-less (see buildSubAgentRuntimeTools).
	wantDefault := []string{"builtin.read_file"}
	gotDefault := runtimeToolTypes(subAgent.Tools)
	if len(gotDefault) != len(wantDefault) {
		t.Fatalf("subagent default tools = %#v, want %#v", gotDefault, wantDefault)
	}
	for i, want := range wantDefault {
		if gotDefault[i] != want {
			t.Fatalf("subagent default tools = %#v, want %#v", gotDefault, wantDefault)
		}
	}
}

func runtimeToolTypes(tools []map[string]any) []string {
	out := make([]string, 0, len(tools))
	for _, tool := range tools {
		value, _ := tool["type"].(string)
		out = append(out, value)
	}
	return out
}
