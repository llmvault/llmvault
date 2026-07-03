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

	mention, ok := bySlug["github-mention"]
	if !ok {
		t.Fatal("github-mention template missing")
	}
	if mention.Integration.Provider != "github-app" || mention.Trigger.Key != "mention" ||
		mention.Trigger.Defaults.Instructions == "" {
		t.Fatalf("github-mention=%+v", mention)
	}
	if len(mention.Plugins.Required) != 1 || mention.Plugins.Required[0] != "github" {
		t.Fatalf("github-mention plugins=%+v", mention.Plugins)
	}
}
