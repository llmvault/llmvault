package testdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/migrations"
)

func ApplyMigrations(t testing.TB, db *gorm.DB) {
	t.Helper()

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql db: %v", err)
	}
	unlock, err := lockMigrationSetup(t.Context(), sqlDB)
	if err != nil {
		t.Fatalf("lock migration setup: %v", err)
	}
	defer unlock(t.Context())

	if current, ok := currentMigrationVersion(t.Context(), sqlDB); ok && current >= latestMigrationVersion {
		missing, err := missingMigratedTables(t.Context(), sqlDB)
		if err == nil && len(missing) == 0 {
			return
		}
		if err != nil {
			t.Logf("test database schema reset required after migration check failed: %v", err)
		} else {
			t.Logf("test database schema reset required; missing baseline tables: %s", strings.Join(missing, ", "))
		}
		resetTestSchema(t, sqlDB)
	}
	if _, err := migrations.Up(t.Context(), sqlDB); err == nil {
		return
	} else if stampInitialSchema(t, sqlDB, err) {
		if _, retryErr := migrations.Up(t.Context(), sqlDB); retryErr != nil {
			t.Fatalf("apply migrations after baseline schema stamp: %v", retryErr)
		} else {
			return
		}
	} else {
		t.Fatalf("apply migrations: %v", err)
	}
}

func resetTestSchema(t testing.TB, db *sql.DB) {
	t.Helper()
	for _, stmt := range []string{
		`DROP SCHEMA IF EXISTS public CASCADE`,
		`CREATE SCHEMA public`,
		`GRANT ALL ON SCHEMA public TO public`,
	} {
		if _, err := db.ExecContext(t.Context(), stmt); err != nil {
			t.Fatalf("reset test schema with %q: %v", stmt, err)
		}
	}
}

func currentMigrationVersion(ctx context.Context, db *sql.DB) (int64, bool) {
	var version int64
	err := db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version_id) FILTER (WHERE is_applied), 0) FROM goose_db_version`).Scan(&version)
	if err == nil {
		return version, true
	}
	if errors.Is(err, sql.ErrNoRows) {
		return 0, true
	}
	return 0, false
}

func lockMigrationSetup(ctx context.Context, db *sql.DB) (func(context.Context), error) {
	previousMaxOpen := db.Stats().MaxOpenConnections
	if previousMaxOpen == 1 {
		db.SetMaxOpenConns(2)
	}

	conn, err := db.Conn(ctx)
	if err != nil {
		if previousMaxOpen == 1 {
			db.SetMaxOpenConns(previousMaxOpen)
		}
		return nil, err
	}
	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_lock(hashtext('hivy_testdb_migrations'))`); err != nil {
		_ = conn.Close()
		if previousMaxOpen == 1 {
			db.SetMaxOpenConns(previousMaxOpen)
		}
		return nil, err
	}
	return func(ctx context.Context) {
		_, _ = conn.ExecContext(ctx, `SELECT pg_advisory_unlock(hashtext('hivy_testdb_migrations'))`)
		_ = conn.Close()
		if previousMaxOpen == 1 {
			db.SetMaxOpenConns(previousMaxOpen)
		}
	}, nil
}

func stampInitialSchema(t testing.TB, db *sql.DB, migrationErr error) bool {
	t.Helper()
	if !strings.Contains(migrationErr.Error(), "already exists") && !strings.Contains(migrationErr.Error(), "SQLSTATE 42P07") {
		return false
	}

	var version int64
	if err := db.QueryRowContext(t.Context(), `SELECT COALESCE(MAX(version_id) FILTER (WHERE is_applied), 0) FROM goose_db_version`).Scan(&version); err != nil {
		return false
	}
	if version != 1 {
		return false
	}

	if missing, err := missingMigratedTables(t.Context(), db); err != nil || len(missing) > 0 {
		if err != nil {
			t.Logf("baseline schema check failed: %v", err)
		} else {
			t.Logf("baseline schema check failed; missing tables: %s", strings.Join(missing, ", "))
		}
		return false
	}

	for version := int64(2); version <= latestMigrationVersion; version++ {
		if _, err := db.ExecContext(t.Context(), `
				INSERT INTO goose_db_version (version_id, is_applied, tstamp)
				SELECT $1, true, now()
				WHERE NOT EXISTS (
					SELECT 1 FROM goose_db_version WHERE version_id = $1 AND is_applied
				)
			`, version); err != nil {
			t.Fatalf("stamp baseline migration version %d: %v", version, err)
		}
	}
	return true
}

func missingMigratedTables(ctx context.Context, db *sql.DB) ([]string, error) {
	const query = `
SELECT table_name
FROM information_schema.tables
WHERE table_schema = current_schema()
	AND table_name = ANY($1)`

	rows, err := db.QueryContext(ctx, query, "{"+strings.Join(migratedTables, ",")+"}")
	if err != nil {
		return nil, fmt.Errorf("query migrated tables: %w", err)
	}
	defer rows.Close()

	found := make(map[string]bool, len(migratedTables))
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			return nil, fmt.Errorf("scan migrated table: %w", err)
		}
		found[table] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate migrated tables: %w", err)
	}

	var missing []string
	for _, table := range migratedTables {
		if !found[table] {
			missing = append(missing, table)
		}
	}
	return missing, nil
}

var migratedTables = []string{
	"api_keys",
	"agent_catalog",
	"agent_channels",
	"audit_log",
	"brand_assets",
	"brands",
	"channel_members",
	"channels",
	"connections",
	"canvas_artifact_files",
	"canvas_artifacts",
	"canvas_projects",
	"credentials",
	"credit_ledger_entries",
	"database_connections",
	"email_verifications",
	"agent_assets",
	"agent_memories",
	"agent_plugin_installs",
	"agent_schedule_runs",
	"agent_schedules",
	"agent_trigger_deliveries",
	"agent_triggers",
	"agents",
	"generations",
	"integrations",
	"oauth_accounts",
	"oauth_exchange_tokens",
	"org_plugin_installs",
	"org_invite_teams",
	"org_invites",
	"org_memberships",
	"orgs",
	"otp_codes",
	"password_resets",
	"plugin_integrations",
	"plugins",
	"plans",
	"rag_embedding_models",
	"rag_external_identities",
	"rag_external_user_groups",
	"rag_index_attempt_errors",
	"rag_index_attempts",
	"rag_public_external_user_groups",
	"rag_search_settings",
	"rag_sources",
	"rag_sync_records",
	"rag_sync_states",
	"rag_user_external_user_groups",
	"refresh_tokens",
	"sandbox_templates",
	"sandbox_warm_slots",
	"sandboxes",
	"sheet_fields",
	"sheet_import_jobs",
	"sheet_operations",
	"sheet_pages",
	"sheet_rows",
	"sheet_views",
	"sheets",
	"slack_thread_events",
	"session_events",
	"session_message_queue",
	"session_participants",
	"session_reflection_states",
	"sessions",
	"skills",
	"subscription_change_quotes",
	"subscriptions",
	"team_members",
	"teams",
	"tokens",
	"tool_usages",
	"usage",
	"users",
}

const latestMigrationVersion = 61
