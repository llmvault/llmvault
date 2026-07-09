package databaseintegration

import (
	"strings"
	"testing"
)

func TestPrepareSQL_DenyWordInsideLiteralAllowed(t *testing.T) {
	if _, err := PrepareSQL(ProviderPostgres,
		`SELECT id FROM users WHERE action = 'delete'`, Policy{}); err != nil {
		t.Fatalf("deny word inside literal must not be denied: %v", err)
	}
}

func TestPrepareSQL_DenyWordInLiteralFilteredIntrospection(t *testing.T) {
	out, err := PrepareSQL(ProviderPostgres,
		`SELECT table_name FROM information_schema.triggers WHERE event_manipulation = 'UPDATE'`,
		restrictedPolicy())
	if err == nil {
		if !strings.Contains(out, "information_schema") {
			t.Fatalf("unexpected rewrite: %s", out)
		}
		return
	}
	if strings.Contains(strings.ToLower(err.Error()), "destructive") {
		t.Fatalf("literal UPDATE must not trip the deny regex: %v", err)
	}
}

func TestPrepareSQL_SemicolonInsideLiteralAllowed(t *testing.T) {
	if _, err := PrepareSQL(ProviderPostgres,
		`SELECT id FROM users WHERE note = 'a;b'`, Policy{}); err != nil {
		t.Fatalf("semicolon inside literal must not be treated as multi-statement: %v", err)
	}
}

func TestPrepareSQL_RealMultiStatementDenied(t *testing.T) {
	if _, err := PrepareSQL(ProviderPostgres, `SELECT 1; SELECT 2`, Policy{}); err == nil {
		t.Fatal("expected real multi-statement to be denied")
	}
}

func TestPrepareSQL_PhantomTableInLiteralNotRejected(t *testing.T) {
	if _, err := PrepareSQL(ProviderPostgres,
		`SELECT id FROM users WHERE msg = 'from secret_table'`,
		Policy{AllowedTables: []string{"users"}}); err != nil {
		t.Fatalf("phantom table reference in literal must not trigger policy rejection: %v", err)
	}
}

func TestPrepareSQL_BlockCommentEvasionDenied(t *testing.T) {
	if _, err := PrepareSQL(ProviderPostgres, `DE/**/LETE FROM x`, Policy{}); err == nil {
		t.Fatal("expected comment-split deny word to be denied")
	}
}

func TestPrepareSQL_DollarQuotedLiteralAllowed(t *testing.T) {
	if _, err := PrepareSQL(ProviderPostgres,
		`SELECT id FROM users WHERE body = $$drop table$$`, Policy{}); err != nil {
		t.Fatalf("dollar-quoted literal must not be denied: %v", err)
	}
}

func TestPrepareSQL_TaggedDollarQuotedLiteralAllowed(t *testing.T) {
	if _, err := PrepareSQL(ProviderPostgres,
		`SELECT id FROM users WHERE body = $tag$delete from x$tag$`, Policy{}); err != nil {
		t.Fatalf("tagged dollar-quoted literal must not be denied: %v", err)
	}
}

func TestPrepareSQL_UnterminatedLiteralFailsClosed(t *testing.T) {
	if _, err := PrepareSQL(ProviderPostgres, `SELECT id FROM users WHERE x = 'abc`, Policy{}); err == nil {
		t.Fatal("expected unterminated literal to fail closed")
	}
}

func TestPrepareSQL_UnterminatedBlockCommentFailsClosed(t *testing.T) {
	if _, err := PrepareSQL(ProviderPostgres, `SELECT id FROM users /* open`, Policy{}); err == nil {
		t.Fatal("expected unterminated block comment to fail closed")
	}
}

func TestPrepareSQL_MySQLBackslashEscapedQuoteAllowed(t *testing.T) {
	if _, err := PrepareSQL(ProviderMySQL,
		`SELECT id FROM users WHERE name = 'O\'Brien; drop'`, Policy{}); err != nil {
		t.Fatalf("mysql backslash escaped quote must be handled: %v", err)
	}
}

func TestPrepareSQL_QuotedIdentifierStillExtracted(t *testing.T) {
	if _, err := PrepareSQL(ProviderPostgres,
		`SELECT id FROM "secret_table"`, Policy{AllowedTables: []string{"users"}}); err == nil {
		t.Fatal("quoted identifier table must still be checked against policy")
	}
}

func TestPrepareSQL_NestedBlockCommentPostgres(t *testing.T) {
	if _, err := PrepareSQL(ProviderPostgres,
		`SELECT id /* outer /* inner */ still */ FROM users`, Policy{}); err != nil {
		t.Fatalf("nested block comment must be handled for postgres: %v", err)
	}
}
