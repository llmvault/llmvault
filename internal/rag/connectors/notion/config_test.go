package notion

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestNormalizeNotionID(t *testing.T) {
	const canonical = "1429989f-e8ac-4eff-bc8f-57f56486db54"
	for _, tc := range []struct {
		name string
		in   string
		want string
	}{
		{"dashless", "1429989fe8ac4effbc8f57f56486db54", canonical},
		{"dashed", "1429989f-e8ac-4eff-bc8f-57f56486db54", canonical},
		{"uppercase dashless", "1429989FE8AC4EFFBC8F57F56486DB54", canonical},
		{"full url", "https://www.notion.so/My-Page-Title-1429989fe8ac4effbc8f57f56486db54", canonical},
		{"url with query", "https://notion.so/1429989fe8ac4effbc8f57f56486db54?pvs=4", canonical},
		{"whitespace", "  1429989fe8ac4effbc8f57f56486db54  ", canonical},
		{"non-uuid passthrough", "page-0", "page-0"},
		{"empty", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeNotionID(tc.in); got != tc.want {
				t.Fatalf("normalizeNotionID(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestLoadConfig_ScopeItemsShape(t *testing.T) {
	raw := json.RawMessage(`{
		"scope": {"items": [
			{"id": "1429989fe8ac4effbc8f57f56486db54"},
			{"id": "1429989f-e8ac-4eff-bc8f-57f56486db54"},
			{"id": "  "}
		]}
	}`)
	cfg, err := LoadConfig(raw)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	// Both ids normalise to the same canonical UUID, so dedup leaves one.
	want := []string{"1429989f-e8ac-4eff-bc8f-57f56486db54"}
	if !reflect.DeepEqual(cfg.RootPageIDs, want) {
		t.Fatalf("RootPageIDs = %v, want %v", cfg.RootPageIDs, want)
	}
	if !cfg.IncludeDatabases {
		t.Fatal("IncludeDatabases should default to true")
	}
}

func TestLoadConfig_TopLevelFallback(t *testing.T) {
	raw := json.RawMessage(`{
		"root_page_ids": ["1429989fe8ac4effbc8f57f56486db54"],
		"include_databases": false
	}`)
	cfg, err := LoadConfig(raw)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if len(cfg.RootPageIDs) != 1 || cfg.RootPageIDs[0] != "1429989f-e8ac-4eff-bc8f-57f56486db54" {
		t.Fatalf("RootPageIDs = %v", cfg.RootPageIDs)
	}
	if cfg.IncludeDatabases {
		t.Fatal("IncludeDatabases=false should be honoured")
	}
}

func TestLoadConfig_ScopeTakesPrecedenceOverTopLevel(t *testing.T) {
	raw := json.RawMessage(`{
		"scope": {"items": [{"id": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}]},
		"root_page_ids": ["bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"]
	}`)
	cfg, err := LoadConfig(raw)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	want := []string{"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"}
	if !reflect.DeepEqual(cfg.RootPageIDs, want) {
		t.Fatalf("RootPageIDs = %v, want %v (scope should win)", cfg.RootPageIDs, want)
	}
}

func TestLoadConfig_EmptyAndNull(t *testing.T) {
	for _, in := range []string{``, `null`, `{}`} {
		cfg, err := LoadConfig(json.RawMessage(in))
		if err != nil {
			t.Fatalf("LoadConfig(%q): %v", in, err)
		}
		if cfg.RootPageIDs != nil {
			t.Fatalf("expected nil RootPageIDs for %q, got %v", in, cfg.RootPageIDs)
		}
		if !cfg.IncludeDatabases {
			t.Fatalf("IncludeDatabases should default true for %q", in)
		}
	}
}
