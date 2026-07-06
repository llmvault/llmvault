package linear

import (
	"testing"

	"github.com/usehivy/hivy/internal/rag/connectors/interfaces"
)

func TestRegistration_LinearKindResolves(t *testing.T) {
	factory, err := interfaces.Lookup(Kind)
	if err != nil {
		t.Fatalf("interfaces.Lookup(%q): %v", Kind, err)
	}
	if factory == nil {
		t.Fatal("Lookup returned nil factory")
	}

	src := newFixtureSource("{}")
	conn, err := factory(src, interfaces.BuildDeps{})
	if err != nil {
		t.Fatalf("factory: %v", err)
	}
	if conn == nil {
		t.Fatal("factory returned nil connector")
	}
	if conn.Kind() != Kind {
		t.Fatalf("Kind() = %q, want %q", conn.Kind(), Kind)
	}
	if _, ok := conn.(*LinearConnector); !ok {
		t.Fatalf("factory returned %T, want *LinearConnector", conn)
	}
}
