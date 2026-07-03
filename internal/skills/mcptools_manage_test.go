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

func TestSkillManagerEnabled(t *testing.T) {
	if skillManagerEnabled(nil) {
		t.Fatal("nil agent must not be enabled")
	}
	if !skillManagerEnabled(&model.Agent{IsDefault: true}) {
		t.Fatal("default agent must be enabled")
	}
	if skillManagerEnabled(&model.Agent{}) {
		t.Fatal("non-default agent without allow-list must not be enabled")
	}
	allowed := &model.Agent{McpToolFilter: &model.ToolFilter{Allow: []string{"create_skill"}}}
	if !skillManagerEnabled(allowed) {
		t.Fatal("allow-listed agent must be enabled")
	}
	other := &model.Agent{McpToolFilter: &model.ToolFilter{Allow: []string{"create_agent"}}}
	if skillManagerEnabled(other) {
		t.Fatal("unrelated allow-list must not enable skill manager")
	}
}

func TestValidateSkillFilePath(t *testing.T) {
	valid := []string{
		"references/api.md",
		"scripts/check.sh",
		"templates/proposal.md",
		"assets/logo.svg",
		"references/nested/deep.md",
	}
	for _, path := range valid {
		if err := validateSkillFilePath(path); err != nil {
			t.Errorf("path %q should be valid: %v", path, err)
		}
	}
	invalid := []string{
		"",
		"SKILL.md",
		"references",
		"references/",
		"/references/api.md",
		"references/../secrets.md",
		"../references/api.md",
		"docs/api.md",
		"references\\api.md",
		"references/./api.md",
	}
	for _, path := range invalid {
		if err := validateSkillFilePath(path); err == nil {
			t.Errorf("path %q should be rejected", path)
		}
	}
}

func TestValidateSkillFieldsRejectsFrontmatterAndBadEnvNames(t *testing.T) {
	if res := validateSkillFields("Name", "Use when testing.", "---\nname: x\n---\nbody", nil, nil); res == nil {
		t.Fatal("content with frontmatter must be rejected")
	}
	if res := validateSkillFields("Name", "Use when testing.", "# Body", nil, []string{"lower_case"}); res == nil {
		t.Fatal("lowercase env var name must be rejected")
	}
	if res := validateSkillFields("Name", "Use when testing.", "# Body", nil, []string{"HIVY_ORG_API_KEY"}); res != nil {
		t.Fatalf("valid fields rejected: %v", toolResultText(res))
	}
	if res := validateSkillFields("Name", "", "# Body", nil, nil); res == nil {
		t.Fatal("empty description must be rejected")
	}
}

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

func TestSkillManagerCreateUpdateArchiveFlow(t *testing.T) {
	db := connectManageTestDB(t)
	ctx := context.Background()

	org := model.Org{ID: uuid.New(), Name: "skill-manager-" + uuid.NewString()[:8], RateLimit: 1000}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", org.ID).Delete(&model.Org{}) })
	token := &model.Token{OrgID: org.ID}

	// Create the org plugin group.
	res, err := handleCreateOrgPlugin(ctx, db, token, createOrgPluginArgs{Name: "Engineering", Description: "Engineering skills"})
	if err != nil {
		t.Fatalf("create org plugin: %v", err)
	}
	created := decodeToolResult(t, res)
	pluginObj := created["plugin"].(map[string]any)
	pluginSlug := pluginObj["slug"].(string)
	if pluginSlug != "engineering" {
		t.Fatalf("plugin slug = %q, want engineering", pluginSlug)
	}
	var install model.OrgPluginInstall
	if err := db.Where("org_id = ? AND revoked_at IS NULL", org.ID).First(&install).Error; err != nil {
		t.Fatalf("org plugin should be installed on the org: %v", err)
	}

	// Duplicate name must be rejected.
	res, _ = handleCreateOrgPlugin(ctx, db, token, createOrgPluginArgs{Name: "Engineering"})
	if res == nil || !res.IsError {
		t.Fatal("duplicate org plugin slug must be rejected")
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
		PluginSlug: globalPlugin.Slug, Name: "X", Description: "Use when testing.", Content: "# X",
	})
	if res == nil || !res.IsError || !strings.Contains(toolResultText(res), "global catalog plugin") {
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

	// The skill resolves through the normal plugin-install entitlement chain.
	agent := model.Agent{
		ID:              uuid.New(),
		OrgID:           &org.ID,
		Name:            "manage-test-agent",
		SandboxSize:     model.DefaultAgentSandboxSize,
		Model:           "test-model",
		Status:          "active",
		Tools:           model.JSON{},
		McpServers:      model.RawJSON("[]"),
		Skills:          model.JSON{},
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	pluginID, _ := uuid.Parse(pluginObj["id"].(string))
	if err := db.Create(&model.AgentPluginInstall{OrgID: org.ID, AgentID: agent.ID, PluginID: pluginID}).Error; err != nil {
		t.Fatalf("attach plugin to agent: %v", err)
	}
	resolved, err := loadAgentPublishedSkills(ctx, db, agent.ID)
	if err != nil {
		t.Fatalf("load agent skills: %v", err)
	}
	if len(resolved) != 1 || resolved[0].Slug != "deploy-checklist" {
		t.Fatalf("agent should resolve the org skill, got %#v", resolved)
	}

	// Update patches content and replaces files.
	newContent := "# Deploy v2\nRun the new script."
	res, err = handleUpdateSkill(ctx, db, token, "http://localhost:3000", updateSkillArgs{
		PluginSlug: pluginSlug,
		Skill:      "deploy-checklist",
		Content:    &newContent,
		Files:      &map[string]string{"scripts/verify.sh": "echo verify\n"},
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
	res, _ = handleArchiveSkill(ctx, db, token, archiveSkillArgs{PluginSlug: pluginSlug, Skill: "deploy-checklist"})
	if res == nil || !res.IsError || !strings.Contains(toolResultText(res), "user_approved") {
		t.Fatalf("archive without approval must instruct the approval flow, got: %s", toolResultText(res))
	}
	res, err = handleArchiveSkill(ctx, db, token, archiveSkillArgs{PluginSlug: pluginSlug, Skill: "deploy-checklist", UserApproved: true})
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
	if len(resolved) != 0 {
		t.Fatalf("archived skill must not resolve, got %#v", resolved)
	}
}
