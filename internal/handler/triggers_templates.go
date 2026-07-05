package handler

import (
	"strings"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/slackapp"
)

const githubAppProvider = "github-app"

// githubToolingProviders are the connection providers that grant an agent
// working GitHub tooling (the `gh` CLI is authenticated from any of these via
// the git-credentials helper). Matches internal/handler/git_credentials.go so
// the install check reflects the same capability the runtime relies on.
var githubToolingProviders = []string{"github-app", "github-app-oauth", "github", "github-pat"}

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
	// requiredProviders, when set, requires the target agent to have a plugin
	// installed that resolves one of these connection providers — otherwise the
	// trigger's playbook could not act (e.g. the agent could not post a reply).
	requiredProviders []string
	// requiredPluginLabel names the plugin in install-rejection messages.
	requiredPluginLabel string
}

var triggerTemplates = []triggerTemplate{
	{
		provider:     slackapp.Provider,
		key:          slackapp.EventReactionAdded,
		resourceType: "slack_channel",
		triggerKeys:  []string{slackapp.EventReactionAdded},
	},
	{
		provider:            githubAppProvider,
		key:                 model.TriggerKeyGitHubIssueMention,
		resourceType:        "github_repo",
		triggerKeys:         model.GitHubIssueMentionEventKeys,
		valueFromResource:   true,
		requiredProviders:   githubToolingProviders,
		requiredPluginLabel: "GitHub",
	},
	{
		provider:            githubAppProvider,
		key:                 model.TriggerKeyGitHubPRMention,
		resourceType:        "github_repo",
		triggerKeys:         model.GitHubPRMentionEventKeys,
		valueFromResource:   true,
		requiredProviders:   githubToolingProviders,
		requiredPluginLabel: "GitHub",
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
