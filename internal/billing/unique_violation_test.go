package billing

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// Idempotency-key collisions must be detected by the pgconn SQLSTATE, not by
// scanning the error message (which false-positives on unrelated errors).
func TestIsUniqueViolation_MatchesStructuredCode(t *testing.T) {
	t.Run("structured 23505 is a unique violation", func(t *testing.T) {
		err := fmt.Errorf("create entry: %w", &pgconn.PgError{Code: "23505", Message: "duplicate key"})
		if !isUniqueViolation(err) {
			t.Fatalf("expected isUniqueViolation(true) for wrapped *pgconn.PgError code 23505")
		}
	})

	t.Run("other pg codes are not unique violations", func(t *testing.T) {
		err := &pgconn.PgError{Code: "23503", Message: "foreign key violation"}
		if isUniqueViolation(err) {
			t.Fatalf("expected isUniqueViolation(false) for code 23503")
		}
	})

	t.Run("plain error mentioning the string is not a unique violation", func(t *testing.T) {
		// Previously this matched on substring and falsely reported true.
		err := errors.New("logged: duplicate key value violates unique constraint elsewhere")
		if isUniqueViolation(err) {
			t.Fatalf("expected isUniqueViolation(false) for a non-PgError message")
		}
	})

	t.Run("nil is not a unique violation", func(t *testing.T) {
		if isUniqueViolation(nil) {
			t.Fatalf("expected isUniqueViolation(false) for nil")
		}
	})
}
