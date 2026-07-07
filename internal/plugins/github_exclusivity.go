package plugins

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

// ErrGitHubIdentityExclusive is returned when attaching a plugin would give an
// agent two GitHub identities. Git credential resolution and sandbox repo
// cloning resolve a single github-app / github-app-code-reviews connection per
// agent (connectionaccess.ResolveAgentProviderAny in
// internal/handler/git_credentials.go and internal/agentruntime/repositories.go),
// so an agent that installs both the GitHub plugin and the GitHub Code Reviews
// plugin has an ambiguous identity — the resolver would silently prefer the
// primary app. We forbid it at every plugin-install write path instead.
var ErrGitHubIdentityExclusive = errors.New("an agent can connect to only one of GitHub or GitHub Code Reviews")

// gitHubIdentityProviders are the two mutually-exclusive GitHub App providers.
// The exclusivity rule is keyed on the *provider* a plugin requires (via the
// plugin_integrations table), not the plugin slug: this ties the guard directly
// to the credential-resolution ambiguity it prevents, and stays correct even if
// a plugin's slug changes or an org-owned plugin ever requires a GitHub
// provider. (Today both providers belong to global plugins — slug matching would
// also work — but provider matching is the more robust invariant.)
var gitHubIdentityProviders = []string{"github-app", "github-app-code-reviews"}

// CheckGitHubIdentityExclusive verifies that the given set of plugin IDs — the
// full set of plugins an agent would have installed — does not require both
// GitHub App providers. Callers that compute an agent's final desired plugin set
// (e.g. a replace) pass that set directly.
func CheckGitHubIdentityExclusive(ctx context.Context, tx *gorm.DB, pluginIDs []uuid.UUID) error {
	if len(pluginIDs) < 2 {
		return nil
	}
	var providers []string
	if err := tx.WithContext(ctx).
		Model(&model.PluginIntegration{}).
		Where("plugin_id IN ? AND kind = ? AND provider IN ?",
			pluginIDs, model.PluginIntegrationKindIntegration, gitHubIdentityProviders).
		Distinct("provider").
		Pluck("provider", &providers).Error; err != nil {
		return fmt.Errorf("check github identity exclusivity: %w", err)
	}
	hasPrimary, hasReviews := false, false
	for _, provider := range providers {
		switch provider {
		case "github-app":
			hasPrimary = true
		case "github-app-code-reviews":
			hasReviews = true
		}
	}
	if hasPrimary && hasReviews {
		return ErrGitHubIdentityExclusive
	}
	return nil
}

// CheckGitHubIdentityExclusiveOnAdd loads the agent's current plugin installs,
// unions the plugins being added, and enforces the single-GitHub-identity rule.
// Use this from incremental (add-one / add-set) install paths; use
// CheckGitHubIdentityExclusive directly from paths that already compute the
// agent's full final plugin set.
func CheckGitHubIdentityExclusiveOnAdd(ctx context.Context, tx *gorm.DB, orgID, agentID uuid.UUID, adding ...uuid.UUID) error {
	if len(adding) == 0 {
		return nil
	}
	var existing []uuid.UUID
	if err := tx.WithContext(ctx).
		Model(&model.AgentPluginInstall{}).
		Where("org_id = ? AND agent_id = ?", orgID, agentID).
		Pluck("plugin_id", &existing).Error; err != nil {
		return fmt.Errorf("load agent plugin installs: %w", err)
	}
	return CheckGitHubIdentityExclusive(ctx, tx, append(existing, adding...))
}
