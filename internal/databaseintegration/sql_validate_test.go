package databaseintegration

import "testing"

func TestValidateSQL_AllowsReadOnlyQuery(t *testing.T) {
	err := ValidateSQL(ProviderPostgres, `SELECT id, email FROM public.users LIMIT 10`, Policy{
		AllowedSchemas: []string{"public"},
		AllowedTables:  []string{"users"},
	})
	if err != nil {
		t.Fatalf("ValidateSQL returned error: %v", err)
	}
}

func TestValidateSQL_DeniesDestructiveQuery(t *testing.T) {
	err := ValidateSQL(ProviderPostgres, `DROP TABLE users`, Policy{})
	if err == nil {
		t.Fatal("expected destructive query to be denied")
	}
}

func TestValidateSQL_DeniesTableOutsidePolicy(t *testing.T) {
	err := ValidateSQL(ProviderPostgres, `SELECT id FROM invoices`, Policy{AllowedTables: []string{"users"}})
	if err == nil {
		t.Fatal("expected table outside policy to be denied")
	}
}

func TestValidateSQL_DeniesMultipleStatements(t *testing.T) {
	err := ValidateSQL(ProviderPostgres, `SELECT id FROM users; SELECT id FROM invoices`, Policy{})
	if err == nil {
		t.Fatal("expected multiple statements to be denied")
	}
}
