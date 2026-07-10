package plugins

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/pluginresolve"
	"github.com/usehivy/hivy/internal/testdb"
)

func connectAutoInstallTestDB(t *testing.T) *gorm.DB {
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

func TestReconcileAutoInstalledInstallsIntoExistingOrgs(t *testing.T) {
	db := connectAutoInstallTestDB(t)
	plugin := model.Plugin{
		ID:       uuid.New(),
		Slug:     "runtime-autoinstall-" + uuid.NewString()[:8],
		Name:     "Runtime Auto Install",
		Status:   model.PluginStatusActive,
		Manifest: model.RawJSON(`{"auto_install":true}`),
	}
	if err := db.Create(&plugin).Error; err != nil {
		t.Fatalf("create plugin: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", plugin.ID).Delete(&model.Plugin{}) })

	org := model.Org{
		ID:        uuid.New(),
		Name:      "runtime-autoinstall-" + uuid.NewString()[:8],
		RateLimit: 1000,
		Active:    true,
	}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", org.ID).Delete(&model.Org{}) })
	team := seedAutoInstallTeam(t, db, org.ID)

	activeAgent := autoInstallTestAgent(org.ID, team.ID, "active")
	defaultAgent := autoInstallTestAgent(org.ID, team.ID, "default")
	defaultAgent.IsDefault = true
	archivedAgent := autoInstallTestAgent(org.ID, team.ID, "archived")
	archivedAgent.Status = "archived"
	if err := db.Create(&[]model.Agent{activeAgent, defaultAgent, archivedAgent}).Error; err != nil {
		t.Fatalf("create agents: %v", err)
	}

	if err := db.Transaction(func(tx *gorm.DB) error {
		return ReconcileAutoInstalled(context.Background(), tx)
	}); err != nil {
		t.Fatalf("reconcile auto-installed plugins: %v", err)
	}

	var orgInstallCount int64
	if err := db.Model(&model.OrgPluginInstall{}).
		Where("org_id = ? AND plugin_id = ? AND revoked_at IS NULL", org.ID, plugin.ID).
		Count(&orgInstallCount).Error; err != nil {
		t.Fatalf("count org install: %v", err)
	}
	if orgInstallCount != 1 {
		t.Fatalf("org install count = %d, want 1", orgInstallCount)
	}

	effectiveAgents, err := pluginresolve.EffectiveAgentIDsForPlugin(context.Background(), db, org.ID, plugin.ID)
	if err != nil {
		t.Fatalf("resolve effective agents: %v", err)
	}
	has := map[uuid.UUID]bool{}
	for _, id := range effectiveAgents {
		has[id] = true
	}
	if !has[activeAgent.ID] {
		t.Fatalf("active agent missing auto-install plugin from effective set")
	}
	if !has[defaultAgent.ID] {
		t.Fatalf("default agent missing auto-install plugin from effective set")
	}
	if has[archivedAgent.ID] {
		t.Fatalf("archived agent must not appear in effective agents")
	}
}

func TestRefreshPluginSkillInstallCountsUsesEffectiveAgents(t *testing.T) {
	db := connectAutoInstallTestDB(t)
	plugin := model.Plugin{
		ID:       uuid.New(),
		Slug:     "count-refresh-" + uuid.NewString()[:8],
		Name:     "Count Refresh",
		Status:   model.PluginStatusActive,
		Manifest: model.RawJSON(`{}`),
	}
	if err := db.Create(&plugin).Error; err != nil {
		t.Fatalf("create plugin: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", plugin.ID).Delete(&model.Plugin{}) })

	skill := model.Skill{
		ID:       uuid.New(),
		Name:     "count-refresh-skill-" + uuid.NewString()[:8],
		Status:   model.SkillStatusPublished,
		PluginID: &plugin.ID,
	}
	if err := db.Create(&skill).Error; err != nil {
		t.Fatalf("create skill: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", skill.ID).Delete(&model.Skill{}) })

	org := model.Org{
		ID:        uuid.New(),
		Name:      "count-refresh-" + uuid.NewString()[:8],
		RateLimit: 1000,
		Active:    true,
	}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", org.ID).Delete(&model.Org{}) })
	team := seedAutoInstallTeam(t, db, org.ID)

	granted := autoInstallTestAgent(org.ID, team.ID, "granted")
	archived := autoInstallTestAgent(org.ID, team.ID, "archived")
	archived.Status = "archived"
	if err := db.Create(&[]model.Agent{granted, archived}).Error; err != nil {
		t.Fatalf("create agents: %v", err)
	}
	if err := db.Create(&model.OrgPluginInstall{ID: uuid.New(), OrgID: org.ID, PluginID: plugin.ID}).Error; err != nil {
		t.Fatalf("create org install: %v", err)
	}
	if err := db.Create(&model.TeamPlugin{OrgID: org.ID, TeamID: team.ID, PluginID: plugin.ID}).Error; err != nil {
		t.Fatalf("grant plugin to team: %v", err)
	}
	t.Cleanup(func() { db.Where("org_id = ?", org.ID).Delete(&model.TeamPlugin{}) })

	if err := db.Transaction(func(tx *gorm.DB) error {
		return RefreshPluginSkillInstallCounts(context.Background(), tx, plugin.ID)
	}); err != nil {
		t.Fatalf("refresh skill install counts: %v", err)
	}

	var stored model.Skill
	if err := db.First(&stored, "id = ?", skill.ID).Error; err != nil {
		t.Fatalf("load skill: %v", err)
	}
	if stored.InstallCount != 1 {
		t.Fatalf("skill install_count = %d, want 1 (one effective non-archived agent)", stored.InstallCount)
	}
	if err := db.Create(&model.AgentPluginOverride{OrgID: org.ID, AgentID: granted.ID, PluginID: plugin.ID}).Error; err != nil {
		t.Fatalf("disable plugin for granted agent: %v", err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		return RefreshPluginSkillInstallCounts(context.Background(), tx, plugin.ID)
	}); err != nil {
		t.Fatalf("refresh skill install counts after override: %v", err)
	}
	if err := db.First(&stored, "id = ?", skill.ID).Error; err != nil {
		t.Fatalf("reload skill: %v", err)
	}
	if stored.InstallCount != 0 {
		t.Fatalf("skill install_count = %d, want 0 after agent override", stored.InstallCount)
	}
}

func seedAutoInstallTeam(t *testing.T, db *gorm.DB, orgID uuid.UUID) model.Team {
	t.Helper()
	team := model.Team{ID: uuid.New(), OrgID: orgID, Name: "autoinstall-team-" + uuid.NewString()[:8]}
	if err := db.Create(&team).Error; err != nil {
		t.Fatalf("create team: %v", err)
	}
	t.Cleanup(func() {
		db.Where("org_id = ?", orgID).Delete(&model.Agent{})
		db.Where("id = ?", team.ID).Delete(&model.Team{})
	})
	return team
}

func autoInstallTestAgent(orgID, teamID uuid.UUID, suffix string) model.Agent {
	return model.Agent{
		ID:            uuid.New(),
		OrgID:         &orgID,
		TeamID:        teamID,
		Name:          fmt.Sprintf("runtime-autoinstall-%s-%s", suffix, uuid.NewString()[:8]),
		SandboxSize:   model.DefaultAgentSandboxSize,
		Model:         "deepseek-v4-flash",
		Status:        "active",
		Tools:         model.JSON{},
		McpServers:    model.RawJSON("[]"),
		Skills:        model.JSON{},
		RuntimeConfig: model.JSON{},
		Permissions:   model.JSON{},
		Resources:     model.JSON{},
	}
}
