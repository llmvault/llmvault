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
		BaseURL:      "https://proxy.test",
		AuthScheme:   "bearer",
		EncryptedKey: []byte("enc"),
		WrappedDEK:   []byte("dek"),
		ProviderID:   "openrouter",
	}
	if err := db.Create(&cred).Error; err != nil {
		t.Fatalf("create credential: %v", err)
	}
	agent := model.Agent{
		OrgID:        &org.ID,
		CredentialID: &cred.ID,
		Name:         "Hivy",
		Model:        DefaultAgentModel,
		Status:       "active",
		Tools:        model.JSON{},
		McpServers:   model.RawJSON("[]"),
		Skills:       model.JSON{},
		Resources:    model.JSON{},
		Permissions:  model.JSON{},
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
