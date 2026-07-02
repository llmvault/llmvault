package plugins

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/usehivy/hivy/internal/model"
)

func TestSyncLocalStoresSkillHumanDescription(t *testing.T) {
	db := connectAutoInstallTestDB(t)
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin transaction: %v", tx.Error)
	}
	t.Cleanup(func() { _ = tx.Rollback() })
	root := t.TempDir()
	writePluginSyncFile(t, filepath.Join(root, "demo", "plugin.json"), `{
		"version": 1,
		"slug": "human-description-demo",
		"name": "Human Description Demo",
		"description": "Demo plugin.",
		"category": "Testing",
		"icon": "test-tube",
		"icon_color": "#111827",
		"developer": "Hivy",
		"plugin_version": "1",
		"enabled": true
	}`)
	writePluginSyncFile(t, filepath.Join(root, "demo", "skills", "drive", "skill.json"), `{
		"name": "drive",
		"description": "Agent-facing upload instructions.",
		"human_description": "User-facing drive summary.",
		"category": "Files",
		"root": "./SKILL.md",
		"tags": ["drive"],
		"files": []
	}`)
	writePluginSyncFile(t, filepath.Join(root, "demo", "skills", "drive", "SKILL.md"), "# Drive\n")

	if _, err := SyncLocal(context.Background(), tx, root); err != nil {
		t.Fatalf("sync local plugins: %v", err)
	}

	var plugin model.Plugin
	if err := tx.Where("slug = ?", "human-description-demo").First(&plugin).Error; err != nil {
		t.Fatalf("load plugin: %v", err)
	}
	var skill model.Skill
	if err := tx.Where("plugin_id = ? AND slug = ?", plugin.ID, "drive").First(&skill).Error; err != nil {
		t.Fatalf("load plugin skill: %v", err)
	}
	if skill.Description == nil || *skill.Description != "Agent-facing upload instructions." {
		t.Fatalf("description = %#v", skill.Description)
	}
	if skill.HumanDescription == nil || *skill.HumanDescription != "User-facing drive summary." {
		t.Fatalf("human_description = %#v", skill.HumanDescription)
	}
}

func writePluginSyncFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
