package agents

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

func TestAgentBuilderToolsRequireTeamPlugin(t *testing.T) {
	db := testDB(t)
	org := testOrg(t, db)
	team := testTeam(t, db, org.ID)

	agent := model.Agent{
		ID:          uuid.New(),
		OrgID:       &org.ID,
		TeamID:      team.ID,
		Name:        "Hivy",
		IsDefault:   true,
		SandboxSize: model.DefaultAgentSandboxSize,
		Model:       "test-model",
		Status:      "active",
		Tools:       model.JSON{},
		McpServers:  model.RawJSON("[]"),
		Skills:      model.JSON{},
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create default agent: %v", err)
	}
	// The shared integration-test database may still have the previous bundled
	// manifest. Keep the test self-contained and prove that only the team's
	// explicit grant exposes the tools by neutralizing that manifest in a rolled
	// back transaction.
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin entitlement transaction: %v", tx.Error)
	}
	t.Cleanup(func() { tx.Rollback() })
	var plugin model.Plugin
	if err := tx.Where("org_id IS NULL AND slug = ?", AgentBuilderPluginSlug).First(&plugin).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		plugin = model.Plugin{ID: uuid.New(), Slug: AgentBuilderPluginSlug, Name: "Agent Builder", Status: model.PluginStatusActive, Manifest: model.RawJSON(`{}`)}
		if err := tx.Create(&plugin).Error; err != nil {
			t.Fatalf("seed agent-builder plugin: %v", err)
		}
	} else if err != nil {
		t.Fatalf("load agent-builder plugin: %v", err)
	}
	if err := tx.Model(&plugin).Update("manifest", model.RawJSON(`{}`)).Error; err != nil {
		t.Fatalf("neutralize agent-builder manifest: %v", err)
	}
	if err := tx.Create(&model.OrgPluginInstall{ID: uuid.New(), OrgID: org.ID, PluginID: plugin.ID}).Error; err != nil {
		t.Fatalf("install agent-builder plugin: %v", err)
	}

	token := &model.Token{
		OrgID: org.ID,
		Meta: model.JSON{
			model.TokenMetaType:    model.TokenTypeAgentProxy,
			model.TokenMetaAgentID: agent.ID.String(),
		},
	}

	withoutPlugin := agentBuilderMCPToolNames(t, tx, token)
	if withoutPlugin[toolCreateAgent] || withoutPlugin[toolListAgents] {
		t.Fatalf("agent-builder tools must not register until the team enables the plugin: %v", withoutPlugin)
	}

	grantTeamPlugin(t, tx, org.ID, team.ID, plugin.ID)
	withPlugin := agentBuilderMCPToolNames(t, tx, token)
	for _, want := range []string{toolListTeamPlugins, toolListAgents, toolGetAgent, toolCreateAgent, toolUpdateAgent} {
		if !withPlugin[want] {
			t.Fatalf("agent-builder tool %q missing after team enablement: %v", want, withPlugin)
		}
	}
}

func agentBuilderMCPToolNames(t *testing.T, db *gorm.DB, token *model.Token) map[string]bool {
	t.Helper()
	ctx := context.Background()
	server := mcp.NewServer(&mcp.Implementation{Name: "hivy-test", Version: "v1"}, nil)
	NewToolsFunc(noopDeps(db), "")(server, token)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("connect server: %v", err)
	}
	defer serverSession.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "agents-mcp-test", Version: "v1"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	defer clientSession.Close()
	tools, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	names := make(map[string]bool, len(tools.Tools))
	for _, tool := range tools.Tools {
		names[tool.Name] = true
	}
	return names
}
