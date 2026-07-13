package skills

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/testdb"
)

func connectManageTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := testdb.DatabaseURL("DATABASE_URL", "HIVY_DATABASE_URL", "TEST_DATABASE_URL")
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect Postgres: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(3)
	sqlDB.SetMaxIdleConns(1)
	testdb.ApplyMigrations(t, db)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

func toolResultText(res *mcp.CallToolResult) string {
	if res == nil || len(res.Content) == 0 {
		return ""
	}
	if text, ok := res.Content[0].(*mcp.TextContent); ok {
		return text.Text
	}
	return ""
}

func decodeToolResult(t *testing.T, res *mcp.CallToolResult) map[string]any {
	t.Helper()
	if res == nil {
		t.Fatal("nil tool result")
	}
	if res.IsError {
		t.Fatalf("tool error: %s", toolResultText(res))
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(toolResultText(res)), &out); err != nil {
		t.Fatalf("decode tool result: %v (%s)", err, toolResultText(res))
	}
	return out
}

func manageTestAgent(orgID, teamID uuid.UUID) model.Agent {
	return model.Agent{
		ID:            uuid.New(),
		OrgID:         &orgID,
		TeamID:        teamID,
		Name:          "manage-test-agent-" + uuid.NewString()[:8],
		SandboxSize:   model.DefaultAgentSandboxSize,
		Model:         "test-model",
		Status:        "active",
		Tools:         model.JSON{},
		McpServers:    model.RawJSON("[]"),
		Skills:        model.JSON{},
		RuntimeConfig: model.JSON{},
		Permissions:   model.JSON{},
		Resources:     model.JSON{},
	}
}

func manageTestToken(orgID, agentID uuid.UUID) *model.Token {
	return &model.Token{
		OrgID: orgID,
		Meta: model.JSON{
			model.TokenMetaType:    model.TokenTypeAgentProxy,
			model.TokenMetaAgentID: agentID.String(),
		},
	}
}

func TestSkillManagerCreateUpdateArchiveFlow(t *testing.T) {
	db := connectManageTestDB(t)
	ctx := context.Background()

	org := model.Org{ID: uuid.New(), Name: "skill-manager-" + uuid.NewString()[:8], RateLimit: 1000}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", org.ID).Delete(&model.Org{}) })
	team := model.Team{ID: uuid.New(), OrgID: org.ID, Name: "manage-test-team-" + uuid.NewString()[:8]}
	if err := db.Create(&team).Error; err != nil {
		t.Fatalf("create team: %v", err)
	}
	agent := manageTestAgent(org.ID, team.ID)
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	token := manageTestToken(org.ID, agent.ID)
	ownerID := seedActor(t, db, org.ID, "owner")
	t.Cleanup(func() {
		db.Where("org_id = ?", org.ID).Delete(&model.TeamPlugin{})
		db.Where("org_id = ?", org.ID).Delete(&model.OrgPluginInstall{})
		db.Where("org_id = ?", org.ID).Delete(&model.Agent{})
		db.Where("id = ?", team.ID).Delete(&model.Team{})
	})

	// Create the team plugin group; it is installed and granted atomically.
	res, err := handleCreateTeamPlugin(ctx, db, token, createTeamPluginArgs{Name: "Engineering", Description: "Engineering skills", HivyActorUserID: ownerID})
	if err != nil {
		t.Fatalf("create team plugin: %v", err)
	}
	created := decodeToolResult(t, res)
	pluginObj := created["plugin"].(map[string]any)
	pluginSlug := pluginObj["slug"].(string)
	if pluginSlug != "engineering" {
		t.Fatalf("plugin slug = %q, want engineering", pluginSlug)
	}
	if pluginObj["team_id"] != team.ID.String() {
		t.Fatalf("plugin team_id = %v, want %s", pluginObj["team_id"], team.ID)
	}
	var storedPlugin model.Plugin
	if err := db.First(&storedPlugin, "id = ?", pluginObj["id"]).Error; err != nil {
		t.Fatalf("load team plugin: %v", err)
	}
	if storedPlugin.TeamID == nil || *storedPlugin.TeamID != team.ID {
		t.Fatalf("stored plugin team_id = %v, want %s", storedPlugin.TeamID, team.ID)
	}
	var install model.OrgPluginInstall
	if err := db.Where("org_id = ? AND revoked_at IS NULL", org.ID).First(&install).Error; err != nil {
		t.Fatalf("team plugin should be installed on the org: %v", err)
	}
	var grant model.TeamPlugin
	if err := db.Where("org_id = ? AND team_id = ? AND plugin_id = ?", org.ID, team.ID, install.PluginID).First(&grant).Error; err != nil {
		t.Fatalf("team plugin should be enabled for its team: %v", err)
	}

	// Duplicate name must be rejected.
	res, _ = handleCreateTeamPlugin(ctx, db, token, createTeamPluginArgs{Name: "Engineering", HivyActorUserID: ownerID})
	if res == nil || !res.IsError {
		t.Fatal("duplicate team plugin slug must be rejected")
	}

	// create_skill rejects global plugins.
	globalPlugin := model.Plugin{
		ID:     uuid.New(),
		Slug:   "manage-global-" + uuid.NewString()[:8],
		Name:   "Global",
		Status: model.PluginStatusActive,
	}
	if err := db.Create(&globalPlugin).Error; err != nil {
		t.Fatalf("create global plugin: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", globalPlugin.ID).Delete(&model.Plugin{}) })
	res, _ = handleCreateSkill(ctx, db, token, "http://localhost:3000", createSkillArgs{
		PluginSlug: globalPlugin.Slug, Name: "X", Description: "Use when testing.", Content: "# X", HivyActorUserID: ownerID,
	})
	if res == nil || !res.IsError || !strings.Contains(toolResultText(res), "catalog plugin") {
		t.Fatalf("create_skill against a global plugin must be rejected, got: %s", toolResultText(res))
	}

	// Create a skill with files and env vars.
	res, err = handleCreateSkill(ctx, db, token, "http://localhost:3000", createSkillArgs{
		PluginSlug:                   pluginSlug,
		Name:                         "Deploy Checklist",
		Description:                  "Use when deploying a service.",
		Content:                      "# Deploy\nRun the script.",
		Files:                        map[string]string{"scripts/check.sh": "#!/bin/bash\necho ok\n"},
		RequiredEnvironmentVariables: []string{"HIVY_ORG_DEPLOY_TOKEN"},
		Tags:                         []string{"deploy", "deploy", " "},
		HivyActorUserID:              ownerID,
	})
	if err != nil {
		t.Fatalf("create skill: %v", err)
	}
	createdSkill := decodeToolResult(t, res)
	if url, _ := createdSkill["environment_settings_url"].(string); url != "http://localhost:3000/w/settings/environments" {
		t.Fatalf("environment_settings_url = %q", url)
	}

	var skill model.Skill
	if err := db.Where("org_id = ? AND slug = ?", org.ID, "deploy-checklist").First(&skill).Error; err != nil {
		t.Fatalf("load created skill: %v", err)
	}
	if skill.Status != model.SkillStatusPublished || skill.SourceType != model.SkillSourceInline {
		t.Fatalf("skill status/source = %q/%q", skill.Status, skill.SourceType)
	}
	bundle, err := decodeSkillBundle(skill)
	if err != nil {
		t.Fatalf("decode bundle: %v", err)
	}
	if bundle.Files["scripts/check.sh"] == "" || len(bundle.References) != 1 {
		t.Fatalf("bundle files not persisted: %#v", bundle)
	}

	// The skill resolves immediately through the team-plugin entitlement chain.
	resolved, err := loadAgentPublishedSkills(ctx, db, agent.ID)
	if err != nil {
		t.Fatalf("load agent skills: %v", err)
	}
	if !resolvedHasSkill(resolved, "deploy-checklist") {
		t.Fatalf("agent should resolve the team-granted skill, got %#v", resolved)
	}

	// Update patches content and replaces files.
	newContent := "# Deploy v2\nRun the new script."
	res, err = handleUpdateSkill(ctx, db, token, "http://localhost:3000", updateSkillArgs{
		PluginSlug:      pluginSlug,
		Skill:           "deploy-checklist",
		Content:         &newContent,
		Files:           &map[string]string{"scripts/verify.sh": "echo verify\n"},
		HivyActorUserID: ownerID,
	})
	if err != nil {
		t.Fatalf("update skill: %v", err)
	}
	decodeToolResult(t, res)
	if err := db.First(&skill, "id = ?", skill.ID).Error; err != nil {
		t.Fatalf("reload skill: %v", err)
	}
	bundle, err = decodeSkillBundle(skill)
	if err != nil {
		t.Fatalf("decode updated bundle: %v", err)
	}
	if bundle.Content != newContent {
		t.Fatalf("content not updated: %q", bundle.Content)
	}
	if _, stale := bundle.Files["scripts/check.sh"]; stale {
		t.Fatal("files must be replaced, old file still present")
	}
	if _, fresh := bundle.Files["scripts/verify.sh"]; !fresh {
		t.Fatal("new file missing after update")
	}

	// Archive requires user approval.
	res, _ = handleArchiveSkill(ctx, db, token, archiveSkillArgs{PluginSlug: pluginSlug, Skill: "deploy-checklist", HivyActorUserID: ownerID})
	if res == nil || !res.IsError || !strings.Contains(toolResultText(res), "user_approved") {
		t.Fatalf("archive without approval must instruct the approval flow, got: %s", toolResultText(res))
	}
	res, err = handleArchiveSkill(ctx, db, token, archiveSkillArgs{PluginSlug: pluginSlug, Skill: "deploy-checklist", UserApproved: true, HivyActorUserID: ownerID})
	if err != nil {
		t.Fatalf("archive skill: %v", err)
	}
	decodeToolResult(t, res)
	if err := db.First(&skill, "id = ?", skill.ID).Error; err != nil {
		t.Fatalf("reload archived skill: %v", err)
	}
	if skill.Status != model.SkillStatusArchived {
		t.Fatalf("skill status = %q, want archived", skill.Status)
	}
	resolved, err = loadAgentPublishedSkills(ctx, db, agent.ID)
	if err != nil {
		t.Fatalf("load agent skills after archive: %v", err)
	}
	if resolvedHasSkill(resolved, "deploy-checklist") {
		t.Fatalf("archived skill must not resolve, got %#v", resolved)
	}
}

func resolvedHasSkill(skills []model.Skill, slug string) bool {
	for _, s := range skills {
		if s.Slug == slug {
			return true
		}
	}
	return false
}
