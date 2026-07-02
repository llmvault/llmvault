package agents

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/testdb"
)

func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := testdb.DatabaseURL()
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect test db: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(3)
	sqlDB.SetMaxIdleConns(1)
	testdb.ApplyMigrations(t, db)
	t.Cleanup(func() { sqlDB.Close() })
	return db
}

func testOrg(t *testing.T, db *gorm.DB) model.Org {
	t.Helper()
	org := model.Org{ID: uuid.New(), Name: "agents-test-" + uuid.NewString()[:8], RateLimit: 1000, Active: true}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	t.Cleanup(func() {
		db.Where("org_id = ?", org.ID).Delete(&model.AgentPluginInstall{})
		db.Where("org_id = ?", org.ID).Delete(&model.OrgPluginInstall{})
		db.Where("org_id = ?", org.ID).Delete(&model.Agent{})
		db.Where("id = ?", org.ID).Delete(&model.Org{})
	})
	return org
}

// seedInstalledPlugin creates an active plugin, installs it for the org, and
// optionally seeds a published skill slug owned by the plugin.
func seedInstalledPlugin(t *testing.T, db *gorm.DB, orgID uuid.UUID, slug, skillSlug string) model.Plugin {
	t.Helper()
	plugin := model.Plugin{
		ID:       uuid.New(),
		Slug:     slug + "-" + uuid.NewString()[:8],
		Name:     "Test " + slug,
		Status:   model.PluginStatusActive,
		Manifest: model.RawJSON(`{}`),
	}
	if err := db.Create(&plugin).Error; err != nil {
		t.Fatalf("create plugin: %v", err)
	}
	install := model.OrgPluginInstall{ID: uuid.New(), OrgID: orgID, PluginID: plugin.ID}
	if err := db.Create(&install).Error; err != nil {
		t.Fatalf("install plugin for org: %v", err)
	}
	if skillSlug != "" {
		skill := model.Skill{
			ID:         uuid.New(),
			PluginID:   &plugin.ID,
			Slug:       skillSlug,
			Name:       skillSlug,
			SourceType: model.SkillSourceInline,
			Status:     model.SkillStatusPublished,
			Bundle:     model.RawJSON(`{}`),
		}
		if err := db.Create(&skill).Error; err != nil {
			t.Fatalf("create skill: %v", err)
		}
		t.Cleanup(func() { db.Where("id = ?", skill.ID).Delete(&model.Skill{}) })
	}
	t.Cleanup(func() { db.Where("id = ?", plugin.ID).Delete(&model.Plugin{}) })
	return plugin
}

func noopDeps(db *gorm.DB) Deps {
	return Deps{DB: db, DefaultModel: "deepseek-v4-flash", ValidateModel: func(context.Context, uuid.UUID, string) error { return nil }}
}

// --- plugin_slug validation ---------------------------------------------------

func TestResolvePluginSlugs_ValidAndUnknown(t *testing.T) {
	db := testDB(t)
	org := testOrg(t, db)
	plugin := seedInstalledPlugin(t, db, org.ID, "crm", "")

	got, err := resolvePluginSlugs(context.Background(), db, org.ID, []string{plugin.Slug})
	if err != nil {
		t.Fatalf("valid slug rejected: %v", err)
	}
	if len(got) != 1 || got[0].ID != plugin.ID {
		t.Fatalf("resolved = %#v, want the seeded plugin", got)
	}

	_, err = resolvePluginSlugs(context.Background(), db, org.ID, []string{"does-not-exist"})
	if err == nil {
		t.Fatal("expected error for unknown plugin slug")
	}
	if !strings.Contains(err.Error(), "does-not-exist") || !strings.Contains(err.Error(), "installed plugins") {
		t.Fatalf("error should name the slug and list installed plugins: %q", err.Error())
	}
	// The installed slug must appear in the guidance.
	if !strings.Contains(err.Error(), plugin.Slug) {
		t.Fatalf("error should list installed slug %q: %q", plugin.Slug, err.Error())
	}
}

// --- skills validation --------------------------------------------------------

func TestValidateSkillSlugs_AgainstOrgAvailable(t *testing.T) {
	db := testDB(t)
	org := testOrg(t, db)
	seedInstalledPlugin(t, db, org.ID, "docs", "summarize-doc")

	got, err := validateSkillSlugs(context.Background(), db, org.ID, []string{"summarize-doc"})
	if err != nil {
		t.Fatalf("valid skill rejected: %v", err)
	}
	if len(got) != 1 || got[0] != "summarize-doc" {
		t.Fatalf("validated skills = %#v, want [summarize-doc]", got)
	}

	_, err = validateSkillSlugs(context.Background(), db, org.ID, []string{"no-such-skill"})
	if err == nil {
		t.Fatal("expected error for unknown skill")
	}
	if !strings.Contains(err.Error(), "no-such-skill") {
		t.Fatalf("error should name the offending skill: %q", err.Error())
	}
	if !strings.Contains(err.Error(), "summarize-doc") {
		t.Fatalf("error should list available skill: %q", err.Error())
	}
}

func TestValidateSkillSlugs_NoSkillsAvailable(t *testing.T) {
	db := testDB(t)
	org := testOrg(t, db)
	_, err := validateSkillSlugs(context.Background(), db, org.ID, []string{"anything"})
	if err == nil || !strings.Contains(err.Error(), "list_org_plugins") {
		t.Fatalf("error should direct caller to list_org_plugins: %v", err)
	}
}

// --- create/update ------------------------------------------------------------

func TestCreateAgent_PersistsToolsSkillsAndPlugin(t *testing.T) {
	db := testDB(t)
	org := testOrg(t, db)
	plugin := seedInstalledPlugin(t, db, org.ID, "search", "web-summary")
	deps := noopDeps(db)

	runtime, mcpAllow, err := SplitTools([]string{"bash", "web_search"})
	if err != nil {
		t.Fatalf("split tools: %v", err)
	}
	agent, err := CreateAgent(context.Background(), deps, org.ID, CreateInput{
		Name:          "Builder Made",
		Instructions:  "Do work.",
		Tools:         runtime,
		McpToolFilter: allowFilter(mcpAllow),
		Skills:        skillsJSON([]string{"web-summary"}),
		PluginIDs:     []uuid.UUID{plugin.ID},
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if agent.Model != "deepseek-v4-flash" {
		t.Fatalf("model = %q, want org default", agent.Model)
	}
	if agent.Tools["bash"] != true {
		t.Fatalf("tools = %#v, want bash", agent.Tools)
	}
	if agent.McpToolFilter == nil || len(agent.McpToolFilter.Allow) != 1 || agent.McpToolFilter.Allow[0] != "web_search" {
		t.Fatalf("mcp filter = %#v, want allow web_search", agent.McpToolFilter)
	}
	if got := agentSkillSlugs(agent); len(got) != 1 || got[0] != "web-summary" {
		t.Fatalf("skills = %#v, want [web-summary]", got)
	}
	var count int64
	db.Model(&model.AgentPluginInstall{}).Where("agent_id = ? AND plugin_id = ?", agent.ID, plugin.ID).Count(&count)
	if count != 1 {
		t.Fatalf("agent plugin installs = %d, want 1", count)
	}
}

func TestCreateAgent_SubAgentSkills(t *testing.T) {
	db := testDB(t)
	org := testOrg(t, db)
	deps := noopDeps(db)

	agent, err := CreateAgent(context.Background(), deps, org.ID, CreateInput{
		Name: "Parent",
		SubAgents: []SubAgentInput{{
			Name:   "Helper",
			Tools:  model.JSON{"grep": true},
			Skills: skillsJSON([]string{"handbook"}),
		}},
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if agent.Tools["subagent_task"] != true {
		t.Fatalf("parent must gain subagent_task: %#v", agent.Tools)
	}
	var sub model.Agent
	if err := db.Where("parent_agent_id = ? AND type = ?", agent.ID, model.AgentTypeSubAgent).First(&sub).Error; err != nil {
		t.Fatalf("load sub-agent: %v", err)
	}
	if got := skillFilterAllow(sub.Skills); len(got) != 1 || got[0] != "handbook" {
		t.Fatalf("sub-agent skills = %#v, want [handbook]", got)
	}
}

func TestUpdateAgent_PatchAndOwnership(t *testing.T) {
	db := testDB(t)
	org := testOrg(t, db)
	other := testOrg(t, db)
	deps := noopDeps(db)

	agent, err := CreateAgent(context.Background(), deps, org.ID, CreateInput{Name: "Patch Me", Instructions: "before"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	newInstr := "after"
	updated, err := UpdateAgent(context.Background(), deps, org.ID, agent.ID, UpdateInput{Instructions: &newInstr})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Instructions == nil || *updated.Instructions != "after" {
		t.Fatalf("instructions = %v, want after", updated.Instructions)
	}
	// Name untouched by the patch.
	if updated.Name != "Patch Me" {
		t.Fatalf("name changed unexpectedly: %q", updated.Name)
	}

	// Cross-org update must fail.
	if _, err := UpdateAgent(context.Background(), deps, other.ID, agent.ID, UpdateInput{Instructions: &newInstr}); err != ErrAgentNotFound {
		t.Fatalf("cross-org update err = %v, want ErrAgentNotFound", err)
	}
}
