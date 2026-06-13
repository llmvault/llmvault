package migrations

import (
	"strings"
	"testing"
)

func TestMicrosandboxFleetMigrationIsEmbedded(t *testing.T) {
	raw, err := migrationFS.ReadFile("sql/000035_microsandbox_fleet.sql")
	if err != nil {
		t.Fatalf("read microsandbox migration: %v", err)
	}
	migration := string(raw)
	for _, table := range []string{
		"microsandbox_runners",
		"microsandbox_sandboxes",
		"microsandbox_sandbox_ports",
		"microsandbox_org_preview_secrets",
		"microsandbox_snapshots",
		"microsandbox_events",
	} {
		if !strings.Contains(migration, "CREATE TABLE IF NOT EXISTS "+table) {
			t.Fatalf("migration does not create %s", table)
		}
	}
}
