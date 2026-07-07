package handler

import (
	"testing"

	"github.com/usehivy/hivy/internal/model"
)

// The GitHub App split adds a pr_mention template for the code-reviews app,
// reusing the primary pr_mention trigger_key and event keys. The connection
// binding differentiates it. There is no issue-mention template for the
// code-reviews app.
func TestTriggerTemplatesGitHubAppSplit(t *testing.T) {
	// Code-reviews app is a supported trigger provider.
	if !triggerProviderSupported(githubAppCodeReviewsProvider) {
		t.Fatalf("provider %q should be supported", githubAppCodeReviewsProvider)
	}

	// pr_mention resolves for the code-reviews app with the shared event keys.
	crPR, ok := resolveTriggerTemplate(githubAppCodeReviewsProvider, model.TriggerKeyGitHubPRMention)
	if !ok {
		t.Fatal("code-reviews pr_mention template not found")
	}
	if crPR.resourceType != "github_repo" || !crPR.valueFromResource {
		t.Fatalf("code-reviews pr_mention template = %+v", crPR)
	}
	if len(crPR.triggerKeys) != len(model.GitHubPRMentionEventKeys) {
		t.Fatalf("code-reviews pr_mention event keys = %v, want %v", crPR.triggerKeys, model.GitHubPRMentionEventKeys)
	}
	for i, key := range model.GitHubPRMentionEventKeys {
		if crPR.triggerKeys[i] != key {
			t.Fatalf("code-reviews pr_mention event keys = %v, want %v", crPR.triggerKeys, model.GitHubPRMentionEventKeys)
		}
	}

	// The code-reviews app has NO issue-mention template (it reviews PRs only).
	if _, ok := resolveTriggerTemplate(githubAppCodeReviewsProvider, model.TriggerKeyGitHubIssueMention); ok {
		t.Fatal("code-reviews app must not expose an issue-mention template")
	}

	// The primary app keeps both mention templates.
	if _, ok := resolveTriggerTemplate(githubAppProvider, model.TriggerKeyGitHubPRMention); !ok {
		t.Fatal("primary pr_mention template missing")
	}
	if _, ok := resolveTriggerTemplate(githubAppProvider, model.TriggerKeyGitHubIssueMention); !ok {
		t.Fatal("primary issue_mention template missing")
	}
}

// The legacy github/github-app-oauth/github-pat providers were removed in the
// split; each template requires exactly its own app so a primary-only agent
// can never receive a code-review trigger and vice versa.
func TestGitHubToolingProvidersNoLegacy(t *testing.T) {
	if got := githubPrimaryToolingProviders; len(got) != 1 || got[0] != githubAppProvider {
		t.Fatalf("githubPrimaryToolingProviders = %v, want [%s]", got, githubAppProvider)
	}
	if got := githubCodeReviewsToolingProviders; len(got) != 1 || got[0] != githubAppCodeReviewsProvider {
		t.Fatalf("githubCodeReviewsToolingProviders = %v, want [%s]", got, githubAppCodeReviewsProvider)
	}
	crPR, ok := resolveTriggerTemplate(githubAppCodeReviewsProvider, model.TriggerKeyGitHubPRMention)
	if !ok {
		t.Fatal("code-reviews pr_mention template missing")
	}
	if len(crPR.requiredProviders) != 1 || crPR.requiredProviders[0] != githubAppCodeReviewsProvider {
		t.Fatalf("code-reviews template requiredProviders = %v, want only %s", crPR.requiredProviders, githubAppCodeReviewsProvider)
	}
	if crPR.requiredPluginLabel != "GitHub Code Reviews" {
		t.Fatalf("code-reviews template plugin label = %q", crPR.requiredPluginLabel)
	}
	primaryPR, ok := resolveTriggerTemplate(githubAppProvider, model.TriggerKeyGitHubPRMention)
	if !ok {
		t.Fatal("primary pr_mention template missing")
	}
	if len(primaryPR.requiredProviders) != 1 || primaryPR.requiredProviders[0] != githubAppProvider {
		t.Fatalf("primary template requiredProviders = %v, want only %s", primaryPR.requiredProviders, githubAppProvider)
	}
}
