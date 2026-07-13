package skills

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

func TestSkillManagerToolsRequireTeamPlugin(t *testing.T) {
	db := connectManageTestDB(t)
	org := model.Org{ID: uuid.New(), Name: "skill-manager-gate-" + uuid.NewString()[:8], RateLimit: 1000}
	team := model.Team{ID: uuid.New(), OrgID: org.ID, Name: "skill-manager-gate-team-" + uuid.NewString()[:8]}
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
	for _, row := range []any{&org, &team, &agent} {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("seed plugin gate fixture %T: %v", row, err)
		}
	}
	t.Cleanup(func() {
		db.Where("id = ?", agent.ID).Delete(&model.Agent{})
		db.Where("id = ?", team.ID).Delete(&model.Team{})
		db.Where("id = ?", org.ID).Delete(&model.Org{})
	})

	// The shared integration-test database may still have the previous bundled
	// manifest. Use a rolled back transaction so this test isolates the new
	// team-entitlement rule without changing that fixture for other tests.
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin entitlement transaction: %v", tx.Error)
	}
	t.Cleanup(func() { tx.Rollback() })
	var plugin model.Plugin
	if err := tx.Where("org_id IS NULL AND slug = ?", skillManagerPluginSlug).First(&plugin).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		plugin = model.Plugin{ID: uuid.New(), Slug: skillManagerPluginSlug, Name: "Skill Manager", Status: model.PluginStatusActive, Manifest: model.RawJSON(`{}`)}
		if err := tx.Create(&plugin).Error; err != nil {
			t.Fatalf("seed skill-manager plugin: %v", err)
		}
	} else if err != nil {
		t.Fatalf("load skill-manager plugin: %v", err)
	}
	if err := tx.Model(&plugin).Update("manifest", model.RawJSON(`{}`)).Error; err != nil {
		t.Fatalf("neutralize skill-manager manifest: %v", err)
	}
	if err := tx.Create(&model.OrgPluginInstall{ID: uuid.New(), OrgID: org.ID, PluginID: plugin.ID}).Error; err != nil {
		t.Fatalf("install skill-manager plugin: %v", err)
	}

	token := &model.Token{OrgID: org.ID, Meta: model.JSON{
		model.TokenMetaType:    model.TokenTypeAgentProxy,
		model.TokenMetaAgentID: agent.ID.String(),
	}}

	withoutPlugin := skillManagerMCPToolNames(t, tx, token)
	if !withoutPlugin["skill_view"] {
		t.Fatal("skill_view must remain universally available")
	}
	if withoutPlugin[toolCreateSkill] || withoutPlugin[toolCreateTeamPlugin] {
		t.Fatalf("skill-manager tools must not register until the team enables the plugin: %v", withoutPlugin)
	}
	if err := tx.Create(&model.TeamPlugin{OrgID: org.ID, TeamID: team.ID, PluginID: plugin.ID}).Error; err != nil {
		t.Fatalf("enable skill-manager plugin for team: %v", err)
	}
	withPlugin := skillManagerMCPToolNames(t, tx, token)
	for _, want := range []string{toolCreateTeamPlugin, toolCreateSkill, toolUpdateSkill, toolArchiveSkill} {
		if !withPlugin[want] {
			t.Fatalf("skill-manager tool %q missing after team enablement: %v", want, withPlugin)
		}
	}
}

func skillManagerMCPToolNames(t *testing.T, db *gorm.DB, token *model.Token) map[string]bool {
	t.Helper()
	ctx := context.Background()
	server := mcp.NewServer(&mcp.Implementation{Name: "hivy-test", Version: "v1"}, nil)
	NewToolsFunc(db, "")(server, token)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("connect server: %v", err)
	}
	defer serverSession.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "skills-mcp-test", Version: "v1"}, nil)
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
