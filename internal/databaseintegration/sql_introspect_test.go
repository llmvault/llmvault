package databaseintegration

import (
	"strings"
	"testing"
)

func restrictedPolicy() Policy {
	return Policy{AllowedSchemas: []string{"public"}, AllowedTables: []string{"users"}}
}

func TestPrepareSQL_IntrospectColumnsFiltered(t *testing.T) {
	out, err := PrepareSQL(ProviderPostgres,
		`SELECT table_name, column_name FROM information_schema.columns`, restrictedPolicy())
	if err != nil {
		t.Fatalf("expected introspection to be allowed: %v", err)
	}
	if !strings.Contains(out, "SELECT * FROM information_schema.columns WHERE") {
		t.Fatalf("expected rewritten subquery, got: %s", out)
	}
	if !strings.Contains(out, "lower(table_schema) IN ('public')") {
		t.Fatalf("expected schema predicate, got: %s", out)
	}
	if !strings.Contains(out, "lower(table_name) = 'users'") {
		t.Fatalf("expected table predicate, got: %s", out)
	}
	if !strings.Contains(out, ") AS columns") {
		t.Fatalf("expected generated alias, got: %s", out)
	}
}

func TestPrepareSQL_IntrospectPreservesExplicitAlias(t *testing.T) {
	out, err := PrepareSQL(ProviderPostgres,
		`SELECT c.table_name FROM information_schema.columns c WHERE c.table_schema = 'public'`,
		restrictedPolicy())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, ") c WHERE") {
		t.Fatalf("expected preserved alias c, got: %s", out)
	}
	if strings.Contains(out, "AS columns") {
		t.Fatalf("should not add alias when one exists: %s", out)
	}
}

func TestPrepareSQL_SelfJoinAliasedFiltered(t *testing.T) {
	out, err := PrepareSQL(ProviderPostgres,
		`SELECT a.column_name FROM information_schema.columns a JOIN information_schema.columns b ON a.table_name = b.table_name`,
		restrictedPolicy())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Count(out, "SELECT * FROM information_schema.columns WHERE") != 2 {
		t.Fatalf("expected both references rewritten, got: %s", out)
	}
	if !strings.Contains(out, ") a JOIN") || !strings.Contains(out, ") b ON") {
		t.Fatalf("expected both aliases preserved, got: %s", out)
	}
}

func TestPrepareSQL_QuotedMixedCaseFiltered(t *testing.T) {
	out, err := PrepareSQL(ProviderPostgres,
		`SELECT table_name FROM "information_schema"."columns"`, restrictedPolicy())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "SELECT * FROM information_schema.columns WHERE") {
		t.Fatalf("expected quoted reference rewritten, got: %s", out)
	}
}

func TestPrepareSQL_WeirdSpacingFiltered(t *testing.T) {
	out, err := PrepareSQL(ProviderPostgres,
		`SELECT table_name FROM information_schema . columns`, restrictedPolicy())
	if err != nil {
		if !strings.Contains(err.Error(), "policy") && !strings.Contains(err.Error(), "introspection") {
			t.Fatalf("unexpected error kind: %v", err)
		}
		return
	}
	if !strings.Contains(out, "SELECT * FROM information_schema.columns WHERE") {
		t.Fatalf("expected weird spacing rewritten or denied, got: %s", out)
	}
}

func TestPrepareSQL_UncuratedPgCatalogDenied(t *testing.T) {
	if _, err := PrepareSQL(ProviderPostgres,
		`SELECT relname FROM pg_class`, restrictedPolicy()); err == nil {
		t.Fatal("expected pg_class to be denied")
	}
	if _, err := PrepareSQL(ProviderPostgres,
		`SELECT nspname FROM pg_catalog.pg_namespace`, restrictedPolicy()); err == nil {
		t.Fatal("expected pg_catalog.pg_namespace to be denied")
	}
}

func TestPrepareSQL_UnionWithDeniedUserTable(t *testing.T) {
	if _, err := PrepareSQL(ProviderPostgres,
		`SELECT table_name FROM information_schema.columns UNION SELECT id FROM secret_table`,
		restrictedPolicy()); err == nil {
		t.Fatal("expected union with denied user table to be denied")
	}
}

func TestPrepareSQL_CatalogTokenOutsideFromDenied(t *testing.T) {
	if _, err := PrepareSQL(ProviderPostgres,
		`SELECT id::information_schema.cardinal_number FROM users`, restrictedPolicy()); err == nil {
		t.Fatal("expected catalog token outside a from/join target to be denied")
	}
}

func TestPrepareSQL_PgTablesFiltered(t *testing.T) {
	out, err := PrepareSQL(ProviderPostgres,
		`SELECT tablename FROM pg_catalog.pg_tables`, restrictedPolicy())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "SELECT * FROM pg_catalog.pg_tables WHERE") {
		t.Fatalf("expected pg_tables rewritten, got: %s", out)
	}
	if !strings.Contains(out, "lower(schemaname) IN ('public')") {
		t.Fatalf("expected schemaname predicate, got: %s", out)
	}
	if !strings.Contains(out, "lower(tablename) = 'users'") {
		t.Fatalf("expected tablename predicate, got: %s", out)
	}
}

func TestPrepareSQL_SchemataUsesSchemaName(t *testing.T) {
	out, err := PrepareSQL(ProviderPostgres,
		`SELECT schema_name FROM information_schema.schemata`,
		Policy{AllowedSchemas: []string{"public", "analytics"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "lower(schema_name) IN ('analytics', 'public')") {
		t.Fatalf("expected schema_name predicate, got: %s", out)
	}
}

func TestPrepareSQL_SchemataOnlyTablesDerivesSchemas(t *testing.T) {
	out, err := PrepareSQL(ProviderPostgres,
		`SELECT schema_name FROM information_schema.schemata`,
		Policy{AllowedTables: []string{"sales.orders"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "lower(schema_name) IN ('sales')") {
		t.Fatalf("expected derived schema, got: %s", out)
	}
}

func TestPrepareSQL_SchemataOnlyBareTablesEmpty(t *testing.T) {
	out, err := PrepareSQL(ProviderPostgres,
		`SELECT schema_name FROM information_schema.schemata`,
		Policy{AllowedTables: []string{"users"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "WHERE 1=0") {
		t.Fatalf("expected fail-closed empty predicate, got: %s", out)
	}
}

func TestPrepareSQL_MixedCatalogAndAllowedUserTable(t *testing.T) {
	out, err := PrepareSQL(ProviderPostgres,
		`SELECT u.id FROM users u JOIN information_schema.columns c ON true`,
		restrictedPolicy())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "FROM users u JOIN (SELECT * FROM information_schema.columns WHERE") {
		t.Fatalf("expected user table kept and catalog rewritten, got: %s", out)
	}
}

func TestPrepareSQL_MixedCatalogAndDeniedUserTable(t *testing.T) {
	if _, err := PrepareSQL(ProviderPostgres,
		`SELECT s.id FROM secret s JOIN information_schema.columns c ON true`,
		restrictedPolicy()); err == nil {
		t.Fatal("expected denied user table in mixed query to fail")
	}
}

func TestPrepareSQL_NoPolicyUnchanged(t *testing.T) {
	query := `SELECT table_name FROM information_schema.columns c JOIN pg_class ON true`
	out, err := PrepareSQL(ProviderPostgres, query, Policy{})
	if err != nil {
		t.Fatalf("unexpected error with no policy: %v", err)
	}
	if out != query {
		t.Fatalf("expected unchanged query with no policy, got: %s", out)
	}
}

func TestPrepareSQL_MySQLInformationSchemaFiltered(t *testing.T) {
	out, err := PrepareSQL(ProviderMySQL,
		`SELECT table_name FROM information_schema.columns`, restrictedPolicy())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "SELECT * FROM information_schema.columns WHERE") {
		t.Fatalf("expected mysql information_schema rewrite, got: %s", out)
	}
}

func TestPrepareSQL_MySQLShowDeniedUnderPolicy(t *testing.T) {
	if _, err := PrepareSQL(ProviderMySQL, `SHOW TABLES`, restrictedPolicy()); err == nil {
		t.Fatal("expected SHOW to be denied under policy")
	}
	if _, err := PrepareSQL(ProviderMySQL, `SHOW TABLES`, Policy{}); err != nil {
		t.Fatalf("expected SHOW allowed without policy: %v", err)
	}
}

func TestPrepareSQL_UnqualifiedCatalogNameFiltered(t *testing.T) {
	out, err := PrepareSQL(ProviderPostgres,
		`SELECT table_name FROM columns`, restrictedPolicy())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "SELECT * FROM information_schema.columns WHERE") {
		t.Fatalf("expected unqualified catalog name rewritten, got: %s", out)
	}
}

func TestPrepareSQL_AllowlistedTableWinsOverCatalogName(t *testing.T) {
	out, err := PrepareSQL(ProviderPostgres,
		`SELECT id FROM columns`, Policy{AllowedTables: []string{"columns"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(out, "information_schema") {
		t.Fatalf("expected user table treatment, got: %s", out)
	}
}
