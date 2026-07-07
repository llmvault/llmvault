package catalog

import (
	"testing"
)

func TestResourceDef(t *testing.T) {
	c := Global()

	// Test GetResourceDef for configured providers
	tests := []struct {
		provider     string
		resourceType string
		wantExists   bool
		wantDef      ResourceDef
	}{
		{
			provider:     "slack",
			resourceType: "slack_channel",
			wantExists:   true,
			wantDef: ResourceDef{
				DisplayName: "Slack Channels",
				IDField:     "id",
				NameField:   "name_normalized",
				Icon:        "hash",
				ListAction:  "/conversations.list",
			},
		},
		{
			provider:     "github-app",
			resourceType: "repository",
			wantExists:   true,
			wantDef: ResourceDef{
				DisplayName: "Repositories",
				Description: "GitHub repositories the AI can access",
				IDField:     "full_name",
				NameField:   "name",
				Icon:        "repo",
				ListAction:  "/installation/repositories",
			},
		},
		{
			provider:     "notion",
			resourceType: "page",
			wantExists:   true,
			wantDef: ResourceDef{
				DisplayName: "Pages",
				Description: "Notion pages the AI can access",
				IDField:     "id",
				NameField:   "title",
				Icon:        "page",
				ListAction:  "/v1/search",
			},
		},
		{
			provider:     "notion",
			resourceType: "database",
			wantExists:   true,
			wantDef: ResourceDef{
				DisplayName: "Databases",
				Description: "Notion databases the AI can query",
				IDField:     "id",
				NameField:   "title",
				Icon:        "database",
				ListAction:  "/v1/search",
			},
		},
		{
			provider:     "unknown",
			resourceType: "channel",
			wantExists:   false,
		},
		{
			provider:     "slack",
			resourceType: "channel",
			wantExists:   false,
		},
		{
			provider:     "slack",
			resourceType: "unknown",
			wantExists:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.provider+"_"+tt.resourceType, func(t *testing.T) {
			def, exists := c.GetResourceDef(tt.provider, tt.resourceType)
			if exists != tt.wantExists {
				t.Errorf("GetResourceDef() exists = %v, want %v", exists, tt.wantExists)
				return
			}
			if !tt.wantExists {
				return
			}
			if def.DisplayName != tt.wantDef.DisplayName {
				t.Errorf("DisplayName = %q, want %q", def.DisplayName, tt.wantDef.DisplayName)
			}
			// Only assert Description when the test supplies one — catalog
			// descriptions are prose and drift over time.
			if tt.wantDef.Description != "" && def.Description != tt.wantDef.Description {
				t.Errorf("Description = %q, want %q", def.Description, tt.wantDef.Description)
			}
			if def.IDField != tt.wantDef.IDField {
				t.Errorf("IDField = %q, want %q", def.IDField, tt.wantDef.IDField)
			}
			if def.NameField != tt.wantDef.NameField {
				t.Errorf("NameField = %q, want %q", def.NameField, tt.wantDef.NameField)
			}
			if def.Icon != tt.wantDef.Icon {
				t.Errorf("Icon = %q, want %q", def.Icon, tt.wantDef.Icon)
			}
			if def.ListAction != tt.wantDef.ListAction {
				t.Errorf("ListAction = %q, want %q", def.ListAction, tt.wantDef.ListAction)
			}
		})
	}
}

func TestListResourceTypes(t *testing.T) {
	c := Global()

	tests := []struct {
		provider  string
		wantCount int
		wantTypes []string
	}{
		{
			provider:  "slack",
			wantCount: 3,
			wantTypes: []string{"slack_channel", "slack_thread", "slack_user"},
		},
		{
			provider:  "github-app",
			wantCount: 10,
			wantTypes: []string{"repository", "issue", "pull_request"},
		},
		{
			provider:  "notion",
			wantCount: 2,
			wantTypes: []string{"page", "database"},
		},
		{
			provider:  "unknown",
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			resources := c.ListResourceTypes(tt.provider)
			if len(resources) != tt.wantCount {
				t.Errorf("ListResourceTypes() count = %d, want %d", len(resources), tt.wantCount)
			}
			for _, wantType := range tt.wantTypes {
				if _, ok := resources[wantType]; !ok {
					t.Errorf("ListResourceTypes() missing type %q", wantType)
				}
			}
		})
	}
}

func TestHasConfigurableResources(t *testing.T) {
	c := Global()

	tests := []struct {
		provider string
		want     bool
	}{
		{"github-app", true},
		{"slack", false},
		{"notion", false},
		{"asana", false},
		{"jira", false},
		{"unknown", false},
	}

	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			got := c.HasConfigurableResources(tt.provider)
			if got != tt.want {
				t.Errorf("HasConfigurableResources() = %v, want %v", got, tt.want)
			}
		})
	}
}

// The GitHub App split gives each app its own triggers file keyed by filename:
// github-app.triggers.json (full catalog) and github-app-code-reviews.triggers.json
// (trimmed mention-only set). Both must resolve exactly, and the code-reviews
// set must be limited to the two mention shapes.
func TestGitHubAppTriggerSplit(t *testing.T) {
	c := Global()

	primary, ok := c.GetProviderTriggers("github-app")
	if !ok {
		t.Fatal("github-app triggers not found (rename from github.triggers.json?)")
	}
	for _, key := range []string{"issue_comment.created", "pull_request.opened", "check_suite.completed", "push"} {
		if _, exists := primary.Triggers[key]; !exists {
			t.Errorf("github-app missing full-catalog trigger %q", key)
		}
	}

	cr, ok := c.GetProviderTriggers("github-app-code-reviews")
	if !ok {
		t.Fatal("github-app-code-reviews triggers not found")
	}
	wantCR := map[string]bool{"issue_comment.created": true, "pull_request.opened": true}
	if len(cr.Triggers) != len(wantCR) {
		t.Fatalf("code-reviews trigger keys = %v, want exactly %v", c.ListTriggers("github-app-code-reviews"), wantCR)
	}
	for key := range wantCR {
		if _, exists := cr.Triggers[key]; !exists {
			t.Errorf("code-reviews missing mention trigger %q", key)
		}
	}
	// The trimmed set must NOT carry the primary-only PR-route/CI events.
	for _, key := range []string{"check_suite.completed", "pull_request_review.submitted", "push"} {
		if _, exists := cr.Triggers[key]; exists {
			t.Errorf("code-reviews unexpectedly carries primary-only trigger %q", key)
		}
	}

	// Exact resolution takes precedence; variant stripping remains only a
	// fallback for names without their own file.
	if _, ok := c.GetTrigger("github-app-code-reviews", "issue_comment.created"); !ok {
		t.Error("GetTrigger should resolve code-reviews issue_comment.created exactly")
	}
	if _, ok := c.GetTrigger("github-app", "check_suite.completed"); !ok {
		t.Error("GetTrigger should resolve github-app check_suite.completed exactly")
	}
}
