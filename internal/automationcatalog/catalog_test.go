package automationcatalog

import "testing"

// LoadTriggers resolves global/triggers relative to the repo root, so this
// validates the real shipped templates.
func TestLoadTriggersIncludesShippedTemplates(t *testing.T) {
	items, err := LoadTriggers("")
	if err != nil {
		t.Fatalf("load triggers: %v", err)
	}
	bySlug := map[string]CatalogItem{}
	for _, item := range items {
		bySlug[item.Slug] = item
	}

	slack, ok := bySlug["slack-reaction"]
	if !ok {
		t.Fatal("slack-reaction template missing")
	}
	if slack.Integration.Provider != "slack" || slack.Trigger.Key != "reaction_added" ||
		slack.Trigger.Defaults.Value == "" || slack.Trigger.Defaults.Instructions == "" {
		t.Fatalf("slack-reaction=%+v", slack)
	}

	for slug, wantKey := range map[string]string{
		"github-issue-mention": "issue_mention",
		"github-pr-mention":    "pr_mention",
	} {
		mention, ok := bySlug[slug]
		if !ok {
			t.Fatalf("%s template missing", slug)
		}
		if mention.Integration.Provider != "github-app" || mention.Trigger.Key != wantKey ||
			mention.Trigger.Defaults.Instructions == "" {
			t.Fatalf("%s=%+v", slug, mention)
		}
		if len(mention.Plugins.Required) != 1 || mention.Plugins.Required[0] != "github" {
			t.Fatalf("%s plugins=%+v", slug, mention.Plugins)
		}
	}
}
