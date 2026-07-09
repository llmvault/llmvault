package pluginresolve

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/testdb"
)

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

type resolveFixture struct {
	db   *gorm.DB
	org  model.Org
	team model.Team
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

func containsID(ids []uuid.UUID, want uuid.UUID) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

func TestEffectivePluginIDs_AutoInstallIncludedForEveryone(t *testing.T) {
	f := newResolveFixture(t)
	auto := f.seedPlugin(t, true, "resolve-auto", `{"auto_install":true}`, model.PluginStatusActive)

	regular := f.seedAgent(t, f.team.ID, false, nil)
	def := f.seedAgent(t, f.team.ID, true, nil)

	for _, agent := range []model.Agent{regular, def} {
		ids, err := EffectivePluginIDs(context.Background(), f.db, agent)
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if !containsID(ids, auto.ID) {
			t.Fatalf("auto-install plugin missing for agent default=%v", agent.IsDefault)
		}
	}
}

func TestEffectivePluginIDs_DefaultAgentPluginOnlyForDefault(t *testing.T) {
	f := newResolveFixture(t)
	da := f.seedPlugin(t, true, "resolve-default-agent", `{"default_agent_install":true}`, model.PluginStatusActive)

	regular := f.seedAgent(t, f.team.ID, false, nil)
	def := f.seedAgent(t, f.team.ID, true, nil)

	regularIDs, err := EffectivePluginIDs(context.Background(), f.db, regular)
	if err != nil {
		t.Fatalf("resolve regular: %v", err)
	}
	if containsID(regularIDs, da.ID) {
		t.Fatalf("default-agent plugin should not be on a non-default agent")
	}

	defIDs, err := EffectivePluginIDs(context.Background(), f.db, def)
	if err != nil {
		t.Fatalf("resolve default: %v", err)
	}
	if !containsID(defIDs, da.ID) {
		t.Fatalf("default-agent plugin missing on the default agent")
	}
}

func TestEffectivePluginIDs_TeamGrantIncludedAndScoped(t *testing.T) {
	f := newResolveFixture(t)
	plugin := f.seedPlugin(t, false, "resolve-team", `{}`, model.PluginStatusActive)
	f.installOrgPlugin(t, plugin.ID, false)
	f.grantTeamPlugin(t, f.team.ID, plugin.ID)

	inTeam := f.seedAgent(t, f.team.ID, false, nil)
	ids, err := EffectivePluginIDs(context.Background(), f.db, inTeam)
	if err != nil {
		t.Fatalf("resolve in-team: %v", err)
	}
	if !containsID(ids, plugin.ID) {
		t.Fatalf("team-granted plugin missing on a team agent")
	}

	otherTeam := model.Team{ID: uuid.New(), OrgID: f.org.ID, Name: "resolve-other-" + uuid.NewString()[:8]}
	if err := f.db.Create(&otherTeam).Error; err != nil {
		t.Fatalf("create other team: %v", err)
	}
	t.Cleanup(func() { f.db.Where("id = ?", otherTeam.ID).Delete(&model.Team{}) })
	outAgent := f.seedAgent(t, otherTeam.ID, false, nil)
	outIDs, err := EffectivePluginIDs(context.Background(), f.db, outAgent)
	if err != nil {
		t.Fatalf("resolve out-team: %v", err)
	}
	if containsID(outIDs, plugin.ID) {
		t.Fatalf("team-granted plugin leaked to an agent in another team")
	}
}

func TestEffectivePluginIDs_OrgUninstalledOrRevokedExcluded(t *testing.T) {
	f := newResolveFixture(t)

	notInstalled := f.seedPlugin(t, false, "resolve-noinstall", `{}`, model.PluginStatusActive)
	f.grantTeamPlugin(t, f.team.ID, notInstalled.ID)

	revoked := f.seedPlugin(t, false, "resolve-revoked", `{}`, model.PluginStatusActive)
	f.installOrgPlugin(t, revoked.ID, true)
	f.grantTeamPlugin(t, f.team.ID, revoked.ID)

	agent := f.seedAgent(t, f.team.ID, false, nil)
	ids, err := EffectivePluginIDs(context.Background(), f.db, agent)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if containsID(ids, notInstalled.ID) {
		t.Fatalf("team grant without an org install must be excluded")
	}
	if containsID(ids, revoked.ID) {
		t.Fatalf("team grant with a revoked org install must be excluded")
	}
}

func TestEffectivePluginIDs_InactivePluginExcluded(t *testing.T) {
	f := newResolveFixture(t)
	inactive := f.seedPlugin(t, false, "resolve-inactive", `{}`, model.PluginStatusArchived)
	f.installOrgPlugin(t, inactive.ID, false)
	f.grantTeamPlugin(t, f.team.ID, inactive.ID)

	agent := f.seedAgent(t, f.team.ID, false, nil)
	ids, err := EffectivePluginIDs(context.Background(), f.db, agent)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if containsID(ids, inactive.ID) {
		t.Fatalf("inactive plugin must be excluded from the effective set")
	}
}
