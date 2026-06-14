package migrations

import (
	"io/fs"
	"strings"
	"testing"
)

func TestMicrosandboxFleetMigrationIsNotEmbeddedInMainApp(t *testing.T) {
	err := fs.WalkDir(migrationFS, "sql", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".sql") {
			return nil
		}
		raw, err := migrationFS.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(raw), "CREATE TABLE IF NOT EXISTS microsandbox_") {
			t.Fatalf("main app migration %s creates microsandbox fleet tables; use internal/microsandbox/migrations instead", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk main app migrations: %v", err)
	}
}
