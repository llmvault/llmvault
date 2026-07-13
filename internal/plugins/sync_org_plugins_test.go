package plugins

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

// Team-owned plugins (and their skills) are created at runtime by the
// skill-manager tools and must be invisible to filesystem sync: never
// archived, never hijacked by a global plugin sharing the slug.
func TestSyncLocalIgnoresTeamOwnedPlugins(t *testing.T) {
	db := connectAutoInstallTestDB(t)
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin transaction: %v", tx.Error)
	}
	t.Cleanup(func() { _ = tx.Rollback() })

	org := model.Org{ID: uuid.New(), Name: "org-plugins-" + uuid.NewString()[:8], RateLimit: 1000}
	if err := tx.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	team := model.Team{ID: uuid.New(), OrgID: org.ID, Name: "sync-team-" + uuid.NewString()[:8]}
	if err := tx.Create(&team).Error; err != nil {
		t.Fatalf("create team: %v", err)
	}
	orgPlugin := model.Plugin{
		ID:       uuid.New(),
		OrgID:    &org.ID,
		TeamID:   &team.ID,
		Slug:     "sync-org-demo",
		Name:     "Sync Org Demo",
		Status:   model.PluginStatusActive,
		Manifest: model.RawJSON(`{"team_plugin":true}`),
	}
	if err := tx.Create(&orgPlugin).Error; err != nil {
		t.Fatalf("create org plugin: %v", err)
	}
	orgSkill := model.Skill{
		ID:         uuid.New(),
		PluginID:   &orgPlugin.ID,
		OrgID:      &org.ID,
		Slug:       "org-demo-skill",
		Name:       "Org Demo Skill",
		SourceType: model.SkillSourceInline,
		Bundle:     model.RawJSON(`{"id":"org-demo-skill","content":"# Demo"}`),
		Status:     model.SkillStatusPublished,
	}
	if err := tx.Create(&orgSkill).Error; err != nil {
		t.Fatalf("create org skill: %v", err)
	}

	// A global plugin with the same slug as the team plugin must sync into its
	// own row, not mutate the team's.
	root := t.TempDir()
	writePluginSyncFile(t, filepath.Join(root, "demo", "plugin.json"), `{
		"version": 1,
		"slug": "sync-org-demo",
		"name": "Global Demo",
		"description": "Global plugin sharing an org plugin slug.",
		"category": "Testing",
		"icon": "test-tube",
		"icon_color": "#111827",
		"developer": "Hivy",
		"plugin_version": "1",
		"enabled": true
	}`)
	writePluginSyncFile(t, filepath.Join(root, "demo", "skills", "demo", "skill.json"), `{
		"name": "demo",
		"description": "Demo skill.",
		"root": "./SKILL.md",
		"files": []
	}`)
	writePluginSyncFile(t, filepath.Join(root, "demo", "skills", "demo", "SKILL.md"), "# Demo\n")

	if _, err := SyncLocal(context.Background(), tx, root); err != nil {
		t.Fatalf("sync local plugins: %v", err)
	}

	var reloadedOrgPlugin model.Plugin
	if err := tx.First(&reloadedOrgPlugin, "id = ?", orgPlugin.ID).Error; err != nil {
		t.Fatalf("reload org plugin: %v", err)
	}
	if reloadedOrgPlugin.Status != model.PluginStatusActive {
		t.Fatalf("org plugin status = %q, want active (sync must not archive org plugins)", reloadedOrgPlugin.Status)
	}
	if reloadedOrgPlugin.Name != "Sync Org Demo" {
		t.Fatalf("org plugin name = %q, want untouched by global sync of the same slug", reloadedOrgPlugin.Name)
	}
	var reloadedOrgSkill model.Skill
	if err := tx.First(&reloadedOrgSkill, "id = ?", orgSkill.ID).Error; err != nil {
		t.Fatalf("reload org skill: %v", err)
	}
	if reloadedOrgSkill.Status != model.SkillStatusPublished {
		t.Fatalf("org skill status = %q, want published", reloadedOrgSkill.Status)
	}

	var globalPlugin model.Plugin
	if err := tx.Where("slug = ? AND org_id IS NULL", "sync-org-demo").First(&globalPlugin).Error; err != nil {
		t.Fatalf("global plugin with shared slug should have been created: %v", err)
	}
	if globalPlugin.ID == orgPlugin.ID {
		t.Fatal("global sync reused the org plugin row")
	}
}
