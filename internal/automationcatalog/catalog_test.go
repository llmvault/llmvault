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
		if len(mention.Connections.Required) != 1 || mention.Connections.Required[0] != "github" {
			t.Fatalf("%s connections=%+v", slug, mention.Connections)
		}
	}

	cr, ok := bySlug["github-code-reviews-pr-mention"]
	if !ok {
		t.Fatal("github-code-reviews-pr-mention template missing")
	}
	if cr.Integration.Provider != "github-app-code-reviews" ||
		cr.Trigger.Key != "pr_mention" || cr.Trigger.Defaults.Instructions == "" {
		t.Fatalf("github-code-reviews-pr-mention=%+v", cr)
	}
	if len(cr.Connections.Required) != 1 || cr.Connections.Required[0] != "github-code-reviews" {
		t.Fatalf("github-code-reviews-pr-mention connections=%+v", cr.Connections)
	}

	crOpened, ok := bySlug["github-code-reviews-pr-opened"]
	if !ok {
		t.Fatal("github-code-reviews-pr-opened template missing")
	}
	if crOpened.Integration.Provider != "github-app-code-reviews" ||
		crOpened.Trigger.Key != "pr_opened" || crOpened.Trigger.Defaults.Instructions == "" {
		t.Fatalf("github-code-reviews-pr-opened=%+v", crOpened)
	}
	if len(crOpened.Connections.Required) != 1 || crOpened.Connections.Required[0] != "github-code-reviews" {
		t.Fatalf("github-code-reviews-pr-opened connections=%+v", crOpened.Connections)
	}
}
