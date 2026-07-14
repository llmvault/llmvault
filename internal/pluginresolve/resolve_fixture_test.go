package pluginresolve

import (
	"testing"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/testdb"
)

type resolveFixture struct {
	db   *gorm.DB
	org  model.Org
	team model.Team
}

func connectResolveTestDB(t *testing.T) *gorm.DB {
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

func newResolveFixture(t *testing.T) resolveFixture {
	t.Helper()
	db := connectResolveTestDB(t)
	org := model.Org{ID: uuid.New(), Name: "resolve-" + uuid.NewString()[:8], RateLimit: 1000, Active: true}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", org.ID).Delete(&model.Org{}) })
	team := model.Team{ID: uuid.New(), OrgID: org.ID, Name: "resolve-team-" + uuid.NewString()[:8]}
	if err := db.Create(&team).Error; err != nil {
		t.Fatalf("create team: %v", err)
	}
	t.Cleanup(func() {
		db.Where("org_id = ?", org.ID).Delete(&model.Agent{})
		db.Where("id = ?", team.ID).Delete(&model.Team{})
	})
	return resolveFixture{db: db, org: org, team: team}
}

func (f resolveFixture) seedAgent(t *testing.T, teamID uuid.UUID, isDefault bool, catalogID *uuid.UUID) model.Agent {
	t.Helper()
	agent := model.Agent{
		ID:             uuid.New(),
		OrgID:          &f.org.ID,
		TeamID:         teamID,
		AgentCatalogID: catalogID,
		Name:           "resolve-agent-" + uuid.NewString()[:8],
		Model:          "deepseek-v4-flash",
		IsDefault:      isDefault,
		Status:         "active",
		Tools:          model.JSON{},
		McpServers:     model.RawJSON("[]"),
		Skills:         model.JSON{},
		RuntimeConfig:  model.JSON{},
		Permissions:    model.JSON{},
		Resources:      model.JSON{},
	}
	if err := f.db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	return agent
}

func (f resolveFixture) seedPlugin(t *testing.T, global bool, slug, manifest, status string) model.Plugin {
	t.Helper()
	plugin := model.Plugin{
		ID:       uuid.New(),
		Slug:     slug + "-" + uuid.NewString()[:8],
		Name:     slug,
		Status:   status,
		Manifest: model.RawJSON(manifest),
	}
	if !global {
		plugin.OrgID = &f.org.ID
		plugin.TeamID = &f.team.ID
	}
	if err := f.db.Create(&plugin).Error; err != nil {
		t.Fatalf("create plugin %q: %v", slug, err)
	}
	t.Cleanup(func() { f.db.Where("id = ?", plugin.ID).Delete(&model.Plugin{}) })
	return plugin
}

func (f resolveFixture) installOrgPlugin(t *testing.T, pluginID uuid.UUID, revoked bool) {
	t.Helper()
	install := model.OrgPluginInstall{ID: uuid.New(), OrgID: f.org.ID, PluginID: pluginID}
	if err := f.db.Create(&install).Error; err != nil {
		t.Fatalf("install org plugin: %v", err)
	}
	if revoked {
		if err := f.db.Model(&model.OrgPluginInstall{}).Where("id = ?", install.ID).UpdateColumn("revoked_at", gorm.Expr("NOW()")).Error; err != nil {
			t.Fatalf("revoke org plugin: %v", err)
		}
	}
	t.Cleanup(func() { f.db.Where("id = ?", install.ID).Delete(&model.OrgPluginInstall{}) })
}

func (f resolveFixture) grantTeamPlugin(t *testing.T, teamID, pluginID uuid.UUID) {
	t.Helper()
	grant := model.TeamPlugin{OrgID: f.org.ID, TeamID: teamID, PluginID: pluginID}
	if err := f.db.Create(&grant).Error; err != nil {
		t.Fatalf("grant team plugin: %v", err)
	}
	t.Cleanup(func() { f.db.Where("team_id = ? AND plugin_id = ?", teamID, pluginID).Delete(&model.TeamPlugin{}) })
}

func (f resolveFixture) disableAgentPlugin(t *testing.T, agentID, pluginID uuid.UUID) {
	t.Helper()
	override := model.AgentPluginOverride{OrgID: f.org.ID, AgentID: agentID, PluginID: pluginID}
	if err := f.db.Create(&override).Error; err != nil {
		t.Fatalf("disable agent plugin: %v", err)
	}
	t.Cleanup(func() {
		f.db.Where("agent_id = ? AND plugin_id = ?", agentID, pluginID).Delete(&model.AgentPluginOverride{})
	})
}

func (f resolveFixture) addIntegration(t *testing.T, pluginID uuid.UUID, provider string) {
	t.Helper()
	integ := model.PluginIntegration{
		PluginID: pluginID,
		Provider: provider,
		Kind:     model.PluginIntegrationKindIntegration,
		Required: true,
	}
	if err := f.db.Create(&integ).Error; err != nil {
		t.Fatalf("create plugin integration: %v", err)
	}
	t.Cleanup(func() { f.db.Where("plugin_id = ?", pluginID).Delete(&model.PluginIntegration{}) })
}

func (f resolveFixture) seedCatalog(t *testing.T, requiredSlugs ...string) model.AgentCatalog {
	t.Helper()
	catalog := model.AgentCatalog{
		ID:              uuid.New(),
		Slug:            "resolve-catalog-" + uuid.NewString()[:8],
		Name:            "Resolve Catalog",
		RequiredPlugins: pq.StringArray(requiredSlugs),
		Status:          model.AgentCatalogStatusActive,
		Manifest:        model.RawJSON(`{}`),
	}
	if err := f.db.Create(&catalog).Error; err != nil {
		t.Fatalf("create catalog: %v", err)
	}
	t.Cleanup(func() { f.db.Where("id = ?", catalog.ID).Delete(&model.AgentCatalog{}) })
	return catalog
}
