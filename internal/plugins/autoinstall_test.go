package plugins

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/model"
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

func TestReconcileAutoInstalledInstallsIntoExistingOrgsAndAgents(t *testing.T) {
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

	activeAgent := autoInstallTestAgent(org.ID, "active")
	defaultAgent := autoInstallTestAgent(org.ID, "default")
	defaultAgent.IsDefault = true
	archivedAgent := autoInstallTestAgent(org.ID, "archived")
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

	var activeInstallCount int64
	if err := db.Model(&model.AgentPluginInstall{}).
		Where("agent_id = ? AND plugin_id = ?", activeAgent.ID, plugin.ID).
		Count(&activeInstallCount).Error; err != nil {
		t.Fatalf("count active agent install: %v", err)
	}
	if activeInstallCount != 1 {
		t.Fatalf("active agent install count = %d, want 1", activeInstallCount)
	}

	var defaultInstallCount int64
	if err := db.Model(&model.AgentPluginInstall{}).
		Where("agent_id = ? AND plugin_id = ?", defaultAgent.ID, plugin.ID).
		Count(&defaultInstallCount).Error; err != nil {
		t.Fatalf("count default agent install: %v", err)
	}
	if defaultInstallCount != 1 {
		t.Fatalf("default agent install count = %d, want 1", defaultInstallCount)
	}

	var archivedInstallCount int64
	if err := db.Model(&model.AgentPluginInstall{}).
		Where("agent_id = ? AND plugin_id = ?", archivedAgent.ID, plugin.ID).
		Count(&archivedInstallCount).Error; err != nil {
		t.Fatalf("count archived agent install: %v", err)
	}
	if archivedInstallCount != 0 {
		t.Fatalf("archived agent install count = %d, want 0", archivedInstallCount)
	}
}

func TestReconcileAutoInstalledBackfillsCatalogRequiredPlugins(t *testing.T) {
	db := connectAutoInstallTestDB(t)
	plugin := model.Plugin{
		ID:       uuid.New(),
		Slug:     "catalog-required-" + uuid.NewString()[:8],
		Name:     "Catalog Required",
		Status:   model.PluginStatusActive,
		Manifest: model.RawJSON(`{}`),
	}
	if err := db.Create(&plugin).Error; err != nil {
		t.Fatalf("create plugin: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", plugin.ID).Delete(&model.Plugin{}) })

	org := model.Org{
		ID:        uuid.New(),
		Name:      "catalog-required-" + uuid.NewString()[:8],
		RateLimit: 1000,
		Active:    true,
	}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", org.ID).Delete(&model.Org{}) })

	catalog := model.AgentCatalog{
		ID:              uuid.New(),
		Slug:            "catalog-required-agent-" + uuid.NewString()[:8],
		Name:            "Catalog Required Agent",
		RequiredPlugins: pq.StringArray{plugin.Slug},
		Status:          model.AgentCatalogStatusActive,
		Manifest:        model.RawJSON(`{}`),
	}
	if err := db.Create(&catalog).Error; err != nil {
		t.Fatalf("create catalog: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", catalog.ID).Delete(&model.AgentCatalog{}) })

	agent := autoInstallTestAgent(org.ID, "catalog-required")
	agent.AgentCatalogID = &catalog.ID
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if err := db.Create(&model.OrgPluginInstall{ID: uuid.New(), OrgID: org.ID, PluginID: plugin.ID}).Error; err != nil {
		t.Fatalf("create org install: %v", err)
	}

	if err := db.Transaction(func(tx *gorm.DB) error {
		return ReconcileAutoInstalled(context.Background(), tx)
	}); err != nil {
		t.Fatalf("reconcile auto-installed plugins: %v", err)
	}

	var installCount int64
	if err := db.Model(&model.AgentPluginInstall{}).
		Where("agent_id = ? AND plugin_id = ?", agent.ID, plugin.ID).
		Count(&installCount).Error; err != nil {
		t.Fatalf("count catalog-required agent install: %v", err)
	}
	if installCount != 1 {
		t.Fatalf("catalog-required install count = %d, want 1", installCount)
	}
}

func autoInstallTestAgent(orgID uuid.UUID, suffix string) model.Agent {
	return model.Agent{
		ID:              uuid.New(),
		OrgID:           &orgID,
		Name:            fmt.Sprintf("runtime-autoinstall-%s-%s", suffix, uuid.NewString()[:8]),
		SandboxSize:     model.DefaultAgentSandboxSize,
		Model:           agentruntime.DefaultAgentModel,
		AvailableModels: []string{agentruntime.DefaultAgentModel},
		Status:          "active",
		Tools:           model.JSON{},
		McpServers:      model.RawJSON("[]"),
		Skills:          model.JSON{},
		RuntimeConfig:   model.JSON{},
		Permissions:     model.JSON{},
		Resources:       model.JSON{},
	}
}
