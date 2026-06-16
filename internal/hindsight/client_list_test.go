package hindsight

import (
	"net/url"
	"strings"
	"testing"
)

func TestListMemoriesPathEncodesFilters(t *testing.T) {
	path, err := listMemoriesPath("org-bank", ListMemoriesOptions{
		Limit:  25,
		Offset: 5,
		TagGroups: []any{map[string]any{
			"tags":  []string{"scope:provider", "provider:github-app"},
			"match": "all_strict",
		}},
		ExcludeTags: []string{"scope:provider", "scope:resource"},
	})
	if err != nil {
		t.Fatalf("build list path: %v", err)
	}
	parts := strings.SplitN(path, "?", 2)
	if len(parts) != 2 {
		t.Fatalf("path missing query: %s", path)
	}
	if parts[0] != "/v1/default/banks/org-bank/memories/list" {
		t.Fatalf("path = %s", parts[0])
	}
	values, err := url.ParseQuery(parts[1])
	if err != nil {
		t.Fatalf("parse query: %v", err)
	}
	for key, want := range map[string]string{"limit": "25", "offset": "5"} {
		if got := values.Get(key); got != want {
			t.Fatalf("%s = %q, want %q", key, got, want)
		}
	}
	if got := values.Get("tag_groups"); !strings.Contains(got, "provider:github-app") {
		t.Fatalf("tag_groups missing provider filter: %s", got)
	}
	if got := values.Get("exclude_tags"); !strings.Contains(got, "scope:resource") {
		t.Fatalf("exclude_tags missing resource exclusion: %s", got)
	}
}
