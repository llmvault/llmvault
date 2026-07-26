package agentruntime

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/model"
)

func TestBuildAgentMCPServer_DisabledWithoutRuntimeToken(t *testing.T) {
	orgID := uuid.New()
	agent := &model.Agent{ID: uuid.New(), OrgID: &orgID}

	if got := buildAgentMCPServer(context.Background(), CompileDeps{}, agent); got != nil {
		t.Fatalf("expected no MCP server without DB/config/token, got %#v", got)
	}
}

func TestProxyTokensCarryAgentMetadata(t *testing.T) {
	db := connectCompileTestDB(t)
	agent := createCompileTokenAgent(t, db)
	deps := CompileDeps{
		DB:         db,
		SigningKey: []byte("test-signing-key-32-bytes-long!!"),
	}

	agentToken, err := MintProxyToken(context.Background(), deps, &agent, uuid.Nil)
	if err != nil {
		t.Fatalf("mint agent token: %v", err)
	}

	var tok model.Token
	if err := db.First(&tok, "jti = ?", agentToken.JTI).Error; err != nil {
		t.Fatalf("load agent token: %v", err)
	}
	if tok.Meta[model.TokenMetaAgentID] != agent.ID.String() {
		t.Fatalf("agent_id = %#v", tok.Meta[model.TokenMetaAgentID])
	}
	if tok.Meta[model.TokenMetaType] != model.TokenTypeAgentProxy {
		t.Fatalf("token type = %#v", tok.Meta[model.TokenMetaType])
	}
	if _, ok := tok.Meta["runtime_mode"]; ok {
		t.Fatalf("runtime_mode should not be present: %#v", tok.Meta)
	}
}

func TestBuildHivyMCPServerSelectsAgentToken(t *testing.T) {
	db := connectCompileTestDB(t)
	agent := createCompileTokenAgent(t, db)
	deps := CompileDeps{
		DB:         db,
		Cfg:        &config.Config{MCPBaseURL: "https://mcp.hivy.test"},
		SigningKey: []byte("test-signing-key-32-bytes-long!!"),
	}

	agentToken, err := MintProxyToken(context.Background(), deps, &agent, uuid.Nil)
	if err != nil {
		t.Fatalf("mint agent token: %v", err)
	}

	agentMCP := buildHivyMCPServer(context.Background(), deps, &agent).(map[string]any)
	if got := agentMCP["url"].(string); !strings.HasSuffix(got, "/"+agentToken.JTI) {
		t.Fatalf("agent mcp url = %q, want suffix %q", got, agentToken.JTI)
	}
	bindings := agentMCP["tool_input_bindings"].([]any)
	if len(bindings) != 3 {
		t.Fatalf("tool input binding count = %d, want 3", len(bindings))
	}
	binding := bindings[0].(map[string]any)
	if binding["tool"] != "send_email" || binding["path_argument"] != "markdown_file_path" || binding["content_argument"] != "markdown" {
		t.Fatalf("unexpected send_email file binding: %#v", binding)
	}
	if binding["max_bytes"] != 1<<20 {
		t.Fatalf("max_bytes = %#v, want 1 MiB", binding["max_bytes"])
	}
	if _, exists := binding["allowed_roots"]; exists {
		t.Fatalf("allowed_roots must not be configurable: %#v", binding)
	}
	for index, tool := range []string{"create_skill", "update_skill"} {
		skillBinding := bindings[index+1].(map[string]any)
		if skillBinding["tool"] != tool ||
			skillBinding["kind"] != "workspace_bundle" ||
			skillBinding["entrypoint_path_argument"] != "entrypoint_file_path" ||
			skillBinding["supporting_file_paths_argument"] != "supporting_file_paths" ||
			skillBinding["entrypoint_content_argument"] != "entrypoint_content" ||
			skillBinding["files_argument"] != "files" {
			t.Fatalf("unexpected %s bundle binding: %#v", tool, skillBinding)
		}
		if skillBinding["entrypoint_filename"] != "SKILL.md" ||
			skillBinding["max_files"] != 256 ||
			skillBinding["max_file_bytes"] != 4<<20 ||
			skillBinding["max_total_bytes"] != 16<<20 {
			t.Fatalf("unexpected %s bundle limits: %#v", tool, skillBinding)
		}
	}
}

func TestBuildAgentRuntimeConfigUpdateWithProxyTokenReusesToken(t *testing.T) {
	db := connectCompileTestDB(t)
	agent := createCompileTokenAgent(t, db)
	deps := CompileDeps{
		DB:         db,
		Cfg:        &config.Config{MCPBaseURL: "https://mcp.hivy.test"},
		SigningKey: []byte("test-signing-key-32-bytes-long!!"),
	}
	agentToken, err := MintProxyToken(context.Background(), deps, &agent, uuid.Nil)
	if err != nil {
		t.Fatalf("mint agent token: %v", err)
	}

	configUpdate, err := BuildAgentRuntimeConfigUpdateWithProxyToken(context.Background(), deps, &agent, nil, "runtime-secret", agentToken)
	if err != nil {
		t.Fatalf("build config with proxy token: %v", err)
	}

	var count int64
	if err := db.Model(&model.Token{}).
		Where("meta->>? = ?", model.TokenMetaAgentID, agent.ID.String()).
		Count(&count).Error; err != nil {
		t.Fatalf("count tokens: %v", err)
	}
	if count != 1 {
		t.Fatalf("token count = %d, want 1", count)
	}
	if got := configUpdate.RuntimeEnv[ProxyAPIKeyEnv]; got != agentToken.Token {
		t.Fatalf("runtime env proxy token = %q, want startup token", got)
	}
	server := configUpdate.Definition.McpServers[0].(map[string]any)
	if got := server["url"].(string); !strings.HasSuffix(got, "/"+agentToken.JTI) {
		t.Fatalf("agent mcp url = %q, want suffix %q", got, agentToken.JTI)
	}
}

func TestUpsertHivyMCPServer_ReplacesExistingHivyServer(t *testing.T) {
	servers := []any{
		map[string]any{"name": "hivy", "url": "old"},
		map[string]any{"name": "linear", "url": "keep"},
	}
	got := upsertHivyMCPServer(servers, map[string]any{"name": "hivy", "url": "new"})

	if len(got) != 2 {
		t.Fatalf("server count = %d, want 2", len(got))
	}
	if got[1].(map[string]any)["url"] != "new" {
		t.Fatalf("hivy server was not replaced: %#v", got)
	}
}

func createCompileTokenAgent(t *testing.T, db *gorm.DB) model.Agent {
	t.Helper()
	org := model.Org{Name: "compile-token-org-" + uuid.NewString(), Active: true}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	cred := model.Credential{
		OrgID:        &org.ID,
		Label:        "compile-token",
		BaseURL:      "https://api.atlascloud.ai/v1",
		AuthScheme:   "bearer",
		EncryptedKey: []byte("enc"),
		WrappedDEK:   []byte("dek"),
		ProviderID:   "atlascloud",
	}
	if err := db.Create(&cred).Error; err != nil {
		t.Fatalf("create credential: %v", err)
	}
	team := model.Team{OrgID: org.ID, Name: "compile-token-team-" + uuid.NewString()[:8]}
	if err := db.Create(&team).Error; err != nil {
		t.Fatalf("create team: %v", err)
	}
	agent := model.Agent{
		OrgID:       &org.ID,
		TeamID:      team.ID,
		Name:        "Hivy",
		Model:       DefaultAgentModel,
		Status:      "active",
		Tools:       model.JSON{},
		McpServers:  model.RawJSON("[]"),
		Skills:      model.JSON{},
		Resources:   model.JSON{},
		Permissions: model.JSON{},
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	t.Cleanup(func() {
		db.Where("org_id = ?", org.ID).Delete(&model.Token{})
		db.Where("id = ?", agent.ID).Delete(&model.Agent{})
		db.Where("id = ?", cred.ID).Delete(&model.Credential{})
		db.Where("id = ?", org.ID).Delete(&model.Org{})
	})
	return agent
}
