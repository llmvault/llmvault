package linear

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestLoadConfig_ScopeEnvelope(t *testing.T) {
	raw := json.RawMessage(`{
		"scope": {"resource_type": "team", "items": [
			{"id": "team-a"},
			{"id": "team-b"}
		]}
	}`)
	cfg, err := LoadConfig(raw)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	want := []string{"team-a", "team-b"}
	if !reflect.DeepEqual(cfg.TeamIDs, want) {
		t.Fatalf("TeamIDs = %v, want %v", cfg.TeamIDs, want)
	}
}

func TestLoadConfig_WrongResourceType(t *testing.T) {
	raw := json.RawMessage(`{
		"scope": {"resource_type": "project", "items": [{"id": "team-a"}]}
	}`)
	if _, err := LoadConfig(raw); err == nil {
		t.Fatal("expected error for wrong resource_type, got nil")
	}
}

func TestLoadConfig_FlatFallback(t *testing.T) {
	raw := json.RawMessage(`{"team_ids": ["team-a", "team-b"]}`)
	cfg, err := LoadConfig(raw)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	want := []string{"team-a", "team-b"}
	if !reflect.DeepEqual(cfg.TeamIDs, want) {
		t.Fatalf("TeamIDs = %v, want %v", cfg.TeamIDs, want)
	}
}

func TestLoadConfig_ScopeTakesPrecedenceOverFlat(t *testing.T) {
	raw := json.RawMessage(`{
		"scope": {"resource_type": "team", "items": [{"id": "scope-team"}]},
		"team_ids": ["flat-team"]
	}`)
	cfg, err := LoadConfig(raw)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	want := []string{"scope-team"}
	if !reflect.DeepEqual(cfg.TeamIDs, want) {
		t.Fatalf("TeamIDs = %v, want %v (scope should win)", cfg.TeamIDs, want)
	}
}

func TestLoadConfig_DedupAndTrim(t *testing.T) {
	// Scope path: trims, dedups preserving order, drops empties.
	scoped := json.RawMessage(`{
		"scope": {"resource_type": "team", "items": [
			{"id": " team-a "},
			{"id": "team-b"},
			{"id": "team-a"},
			{"id": "  "}
		]}
	}`)
	cfg, err := LoadConfig(scoped)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	want := []string{"team-a", "team-b"}
	if !reflect.DeepEqual(cfg.TeamIDs, want) {
		t.Fatalf("scope TeamIDs = %v, want %v", cfg.TeamIDs, want)
	}

	// Flat path: same normalisation.
	flat := json.RawMessage(`{"team_ids": [" team-a ", "team-b", "team-a", ""]}`)
	cfg, err = LoadConfig(flat)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if !reflect.DeepEqual(cfg.TeamIDs, want) {
		t.Fatalf("flat TeamIDs = %v, want %v", cfg.TeamIDs, want)
	}
}

func TestLoadConfig_EmptyIsNilNoError(t *testing.T) {
	cases := []string{
		``,
		`null`,
		`{}`,
		`{"scope": {"resource_type": "team", "items": []}}`,
		`{"team_ids": []}`,
	}
	for _, in := range cases {
		cfg, err := LoadConfig(json.RawMessage(in))
		if err != nil {
			t.Fatalf("LoadConfig(%q): unexpected error %v", in, err)
		}
		if cfg.TeamIDs != nil {
			t.Fatalf("LoadConfig(%q): expected nil TeamIDs, got %v", in, cfg.TeamIDs)
		}
	}
}
