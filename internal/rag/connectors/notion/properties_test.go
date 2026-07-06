package notion

import (
	"strings"
	"testing"
)

// People-type properties store the display name alongside the "type"
// tag; it must be captured before descending into the person sub-object.
func TestPropertiesToStr_PeopleNames(t *testing.T) {
	props := map[string]any{
		"Members": map[string]any{
			"type": "people",
			"people": []any{
				map[string]any{"object": "user", "id": "u1", "name": "Arturo Martinez", "type": "person", "person": map[string]any{"email": "a@x"}},
				map[string]any{"object": "user", "id": "u2", "name": "Jane Smith", "type": "person", "person": map[string]any{}},
			},
		},
	}
	got := propertiesToStr(props)
	for _, want := range []string{"Arturo Martinez", "Jane Smith"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in %q", want, got)
		}
	}
}

func TestPropertiesToStr_BotUserName(t *testing.T) {
	props := map[string]any{
		"Created By": map[string]any{
			"type": "people",
			"people": []any{
				map[string]any{"object": "user", "id": "b1", "name": "Hivy Integration", "type": "bot", "bot": map[string]any{}},
			},
		},
	}
	if got := propertiesToStr(props); !strings.Contains(got, "Hivy Integration") {
		t.Fatalf("expected bot name in %q", got)
	}
}

func TestPropertiesToStr_SelectStatusAndDate(t *testing.T) {
	props := map[string]any{
		"Status":   map[string]any{"type": "status", "status": map[string]any{"name": "In Progress"}},
		"Priority": map[string]any{"type": "select", "select": map[string]any{"name": "High"}},
		"Window":   map[string]any{"type": "date", "date": map[string]any{"start": "2026-01-01", "end": "2026-01-02"}},
	}
	got := propertiesToStr(props)
	for _, want := range []string{"Status: In Progress", "Priority: High", "Window: 2026-01-01 - 2026-01-02"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in %q", want, got)
		}
	}
}

// An unset property (empty innermost value) contributes nothing.
func TestPropertiesToStr_SkipsEmpty(t *testing.T) {
	props := map[string]any{
		"Empty":  map[string]any{"type": "rich_text", "rich_text": []any{}},
		"Filled": map[string]any{"type": "select", "select": map[string]any{"name": "X"}},
	}
	got := propertiesToStr(props)
	if strings.Contains(got, "Empty") {
		t.Fatalf("empty property should be skipped, got %q", got)
	}
	if !strings.Contains(got, "Filled: X") {
		t.Fatalf("expected Filled: X in %q", got)
	}
}
