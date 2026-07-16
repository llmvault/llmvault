package handler

import (
	"strings"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/slackapp"
)

const (
	githubAppProvider            = "github-app"
	githubAppCodeReviewsProvider = "github-app-code-reviews"
)

// Per-app tooling requirements: a trigger's playbook runs under the identity
// of ONE GitHub App, so the target agent must resolve that app's connection
// through its granted connection (github vs github-code-reviews) — resolved with
// the same ResolveAgentProviderAny call the runtime's git-credentials helper
// uses. A primary-app agent must not receive code-review triggers or vice
// versa. The legacy github/github-app-oauth/github-pat providers were removed
// in the GitHub App split.
var (
	githubPrimaryToolingProviders     = []string{githubAppProvider}
	githubCodeReviewsToolingProviders = []string{githubAppCodeReviewsProvider}
)

// triggerTemplate describes a curated installable trigger. Delivery code is
// provider-specific and lives with the webhook pipeline; this registry only
// owns install validation and the stored row shape.
type triggerTemplate struct {
	provider     string
	key          string
	resourceType string
	triggerKeys  []string
	// valueFromResource derives trigger_value from the external resource key
	// instead of user input (e.g. the repo full name for GitHub mentions).
	valueFromResource bool
	// requiredProviders, when set, requires the target agent to have a granted
	// connection for one of these providers — otherwise the
	// trigger's playbook could not act (e.g. the agent could not post a reply).
	requiredProviders []string
	// requiredConnectionLabel names the connection in rejection messages.
	requiredConnectionLabel string
}

var triggerTemplates = []triggerTemplate{
	{
		provider:     slackapp.Provider,
		key:          slackapp.EventReactionAdded,
		resourceType: "slack_channel",
		triggerKeys:  []string{slackapp.EventReactionAdded},
	},
	{
		provider:                githubAppProvider,
		key:                     model.TriggerKeyGitHubIssueMention,
		resourceType:            "github_repo",
		triggerKeys:             model.GitHubIssueMentionEventKeys,
		valueFromResource:       true,
		requiredProviders:       githubPrimaryToolingProviders,
		requiredConnectionLabel: "GitHub",
	},
	{
		provider:                githubAppProvider,
		key:                     model.TriggerKeyGitHubPRMention,
		resourceType:            "github_repo",
		triggerKeys:             model.GitHubPRMentionEventKeys,
		valueFromResource:       true,
		requiredProviders:       githubPrimaryToolingProviders,
		requiredConnectionLabel: "GitHub",
	},
	// The code-reviews app answers @usehivy-reviews on pull requests. It reuses
	// the pr_mention trigger_key and PR mention event keys — the connection
	// binding (a github-app-code-reviews connection) is what differentiates it
	// from the primary pr_mention template above. There is deliberately no
	// issue-mention template for the code-reviews app: it reviews PRs only.
	{
		provider:                githubAppCodeReviewsProvider,
		key:                     model.TriggerKeyGitHubPRMention,
		resourceType:            "github_repo",
		triggerKeys:             model.GitHubPRMentionEventKeys,
		valueFromResource:       true,
		requiredProviders:       githubCodeReviewsToolingProviders,
		requiredConnectionLabel: "GitHub Code Reviews",
	},
	// Auto-review every new pull request. Code-reviews app only — the primary
	// app must never carry this key, so a build agent's PRs are reviewed by
	// usehivy-reviews, not re-run through the primary app. It fires on
	// pull_request.opened with no mention, so unlike pr_mention it subscribes to
	// just that one event key.
	{
		provider:                githubAppCodeReviewsProvider,
		key:                     model.TriggerKeyGitHubPROpened,
		resourceType:            "github_repo",
		triggerKeys:             model.GitHubPROpenedEventKeys,
		valueFromResource:       true,
		requiredProviders:       githubCodeReviewsToolingProviders,
		requiredConnectionLabel: "GitHub Code Reviews",
	},
}

func resolveTriggerTemplate(provider, key string) (triggerTemplate, bool) {
	for _, tpl := range triggerTemplates {
		if tpl.provider == provider && tpl.key == key {
			return tpl, true
		}
	}
	return triggerTemplate{}, false
}

func triggerProviderSupported(provider string) bool {
	for _, tpl := range triggerTemplates {
		if tpl.provider == provider {
			return true
		}
	}
	return false
}

// defaultTriggerKey preserves the legacy install payloads that omitted the
// trigger key and implied the provider's only template.
func defaultTriggerKey(provider, key string) string {
	if key != "" {
		return key
	}
	if provider == slackapp.Provider {
		return slackapp.EventReactionAdded
	}
	return key
}

func (t triggerTemplate) triggerValue(userValue, resourceKey string) string {
	if t.valueFromResource {
		return strings.ToLower(strings.TrimSpace(resourceKey))
	}
	return normalizeTriggerValue(userValue)
}
