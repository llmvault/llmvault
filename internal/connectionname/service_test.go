package connectionname

import (
	"errors"
	"testing"
)

func TestNormalizeConnectionName(t *testing.T) {
	identity, err := Normalize("  Reporting   Database  ")
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
	if identity.Name != "Reporting Database" || identity.Slug != "reporting-database" || identity.NeedsName {
		t.Fatalf("Normalize() = %#v", identity)
	}
}

func TestNormalizeConnectionNameRejectsEmptySlug(t *testing.T) {
	_, err := Normalize("---")
	if !errors.Is(err, ErrInvalidName) {
		t.Fatalf("Normalize() error = %v, want ErrInvalidName", err)
	}
}
