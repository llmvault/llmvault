package plugins

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/usehivy/hivy/internal/model"
)

// The Apps/Sheets manifest transition must not silently remove either plugin
// from an existing Pedro/Ricky-style catalog team. Exercise the shipped SQL in
// a rolled-back transaction so the test validates the actual production
// migration without leaking fixture rows into the shared test database.
func TestAppsSheetsTransitionProvisionsExistingRequiredCatalogTeam(t *testing.T) {
	db := connectAutoInstallTestDB(t)
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin migration transaction: %v", tx.Error)
	}
	t.Cleanup(func() { tx.Rollback() })
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	if _, err := SyncLocal(context.Background(), tx, filepath.Join(filepath.Dir(file), "..", "..", "global", "plugins")); err != nil {
		t.Fatalf("sync global plugin fixtures: %v", err)
	}

	var plugins []model.Plugin
	if err := tx.Where("org_id IS NULL AND slug IN ? AND status = ?", []string{"apps", "sheets"}, model.PluginStatusActive).Find(&plugins).Error; err != nil {
		t.Fatalf("load apps and sheets plugins: %v", err)
	}
	if len(plugins) != 2 {
		t.Fatalf("loaded %d apps/sheets plugins, want 2", len(plugins))
	}

	org := model.Org{ID: uuid.New(), Name: "apps-sheets-transition-" + uuid.NewString()[:8], RateLimit: 1000, Active: true}
	team := model.Team{ID: uuid.New(), OrgID: org.ID, Name: "apps-sheets-transition-team-" + uuid.NewString()[:8]}
	catalog := model.AgentCatalog{
		ID:              uuid.New(),
		Slug:            "apps-sheets-transition-" + uuid.NewString()[:8],
		Name:            "Apps and Sheets Specialist",
		Model:           "test-model",
		Status:          model.AgentCatalogStatusActive,
		RequiredPlugins: pq.StringArray{"apps", "sheets"},
		Tools:           model.JSON{},
		SubAgents:       model.RawJSON(`{}`),
		Manifest:        model.RawJSON(`{}`),
	}
	agent := autoInstallTestAgent(org.ID, team.ID, "apps-sheets-transition")
	agent.AgentCatalogID = &catalog.ID
	for _, row := range []any{&org, &team, &catalog, &agent} {
		if err := tx.Create(row).Error; err != nil {
			t.Fatalf("seed transition fixture %T: %v", row, err)
		}
	}

	migration, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "migrations", "sql", "000089_provision_required_apps_and_sheets.sql"))
	if err != nil {
		t.Fatalf("read transition migration: %v", err)
	}
	if err := tx.Exec(string(migration)).Error; err != nil {
		t.Fatalf("run transition migration: %v", err)
	}

	for _, plugin := range plugins {
		var installCount, grantCount int64
		if err := tx.Model(&model.OrgPluginInstall{}).Where("org_id = ? AND plugin_id = ? AND revoked_at IS NULL", org.ID, plugin.ID).Count(&installCount).Error; err != nil {
			t.Fatalf("count %s org install: %v", plugin.Slug, err)
		}
		if err := tx.Model(&model.TeamPlugin{}).Where("org_id = ? AND team_id = ? AND plugin_id = ?", org.ID, team.ID, plugin.ID).Count(&grantCount).Error; err != nil {
			t.Fatalf("count %s team grant: %v", plugin.Slug, err)
		}
		if installCount != 1 || grantCount != 1 {
			t.Fatalf("%s transition install/grant = %d/%d, want 1/1", plugin.Slug, installCount, grantCount)
		}
	}
}
