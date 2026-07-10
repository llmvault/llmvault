package pluginresolve

import (
	"context"
	"fmt"
	"sort"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

// gitHubPairProviders are the two mutually-exclusive GitHub App providers. An
// agent's effective set never carries both: the pair rule keeps exactly one.
var gitHubPairProviders = []string{"github-app", "github-app-code-reviews"}

// EffectivePluginIDs resolves the plugins an agent effectively has: the global
// auto-install set, the default-agent set when the agent is its team's default,
// and the agent's team grants intersected with the org's active installs minus
// any per-agent disabled overrides. The GitHub identity pair rule is then
// applied so at most one GitHub App survives.
func EffectivePluginIDs(ctx context.Context, db *gorm.DB, agent model.Agent) ([]uuid.UUID, error) {
	set := map[uuid.UUID]bool{}

	autoPlugins, err := autoInstallPlugins(ctx, db)
	if err != nil {
		return nil, err
	}
	for _, plugin := range autoPlugins {
		set[plugin.ID] = true
	}

	if agent.IsDefault {
		defaultPlugins, err := defaultAgentInstallPlugins(ctx, db)
		if err != nil {
			return nil, err
		}
		for _, plugin := range defaultPlugins {
			set[plugin.ID] = true
		}
	}

	if agent.TeamID != uuid.Nil {
		teamPluginIDs, err := teamGrantedPluginIDs(ctx, db, agent.TeamID)
		if err != nil {
			return nil, err
		}
		disabled, err := disabledTeamPluginIDs(ctx, db, agent.ID, teamPluginIDs)
		if err != nil {
			return nil, err
		}
		for _, id := range teamPluginIDs {
			if !disabled[id] {
				set[id] = true
			}
		}
	}

	if err := applyGitHubPairRule(ctx, db, agent, set); err != nil {
		return nil, err
	}

	ids := make([]uuid.UUID, 0, len(set))
	for id := range set {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i].String() < ids[j].String() })
	return ids, nil
}

// EffectivePluginSlugs resolves an agent's effective plugins and returns their
// slugs, sorted.
func EffectivePluginSlugs(ctx context.Context, db *gorm.DB, agent model.Agent) ([]string, error) {
	ids, err := EffectivePluginIDs(ctx, db, agent)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return []string{}, nil
	}
	var slugs []string
	if err := db.WithContext(ctx).
		Model(&model.Plugin{}).
		Where("id IN ?", ids).
		Order("slug ASC").
		Pluck("slug", &slugs).Error; err != nil {
		return nil, fmt.Errorf("load effective plugin slugs: %w", err)
	}
	return slugs, nil
}

// AgentHasPluginSlug reports whether an agent's effective set contains a plugin
// with the given slug.
func AgentHasPluginSlug(ctx context.Context, db *gorm.DB, agent model.Agent, slug string) (bool, error) {
	slugs, err := EffectivePluginSlugs(ctx, db, agent)
	if err != nil {
		return false, err
	}
	for _, s := range slugs {
		if s == slug {
			return true, nil
		}
	}
	return false, nil
}

// EffectiveAgentIDsForPlugin returns the org's non-archived agents whose
// effective set contains the plugin. The pair rule is applied per agent, so a
// GitHub plugin dropped for one agent is excluded from its result.
func EffectiveAgentIDsForPlugin(ctx context.Context, db *gorm.DB, orgID, pluginID uuid.UUID) ([]uuid.UUID, error) {
	if orgID == uuid.Nil || pluginID == uuid.Nil {
		return []uuid.UUID{}, nil
	}
	var agents []model.Agent
	if err := db.WithContext(ctx).
		Where("org_id = ? AND status <> ?", orgID, "archived").
		Order("created_at ASC").
		Find(&agents).Error; err != nil {
		return nil, fmt.Errorf("load org agents: %w", err)
	}
	out := make([]uuid.UUID, 0, len(agents))
	for _, agent := range agents {
		ids, err := EffectivePluginIDs(ctx, db, agent)
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			if id == pluginID {
				out = append(out, agent.ID)
				break
			}
		}
	}
	return out, nil
}

func teamGrantedPluginIDs(ctx context.Context, db *gorm.DB, teamID uuid.UUID) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	if err := db.WithContext(ctx).
		Table("team_plugins tp").
		Joins("JOIN org_plugin_installs opi ON opi.org_id = tp.org_id AND opi.plugin_id = tp.plugin_id AND opi.revoked_at IS NULL").
		Joins("JOIN plugins p ON p.id = tp.plugin_id AND p.status = ?", model.PluginStatusActive).
		Where("tp.team_id = ?", teamID).
		Distinct("tp.plugin_id").
		Pluck("tp.plugin_id", &ids).Error; err != nil {
		return nil, fmt.Errorf("load team plugin grants: %w", err)
	}
	return ids, nil
}

// disabledTeamPluginIDs returns disabled overrides limited to the team-granted
// candidates that are being resolved. Keeping dormant rows when a team plugin
// is revoked means a later re-enable does not unexpectedly restore it for an
// agent the user explicitly opted out of.
func disabledTeamPluginIDs(ctx context.Context, db *gorm.DB, agentID uuid.UUID, teamPluginIDs []uuid.UUID) (map[uuid.UUID]bool, error) {
	disabled := make(map[uuid.UUID]bool)
	if agentID == uuid.Nil || len(teamPluginIDs) == 0 {
		return disabled, nil
	}
	var ids []uuid.UUID
	if err := db.WithContext(ctx).
		Model(&model.AgentPluginOverride{}).
		Where("agent_id = ? AND plugin_id IN ?", agentID, teamPluginIDs).
		Pluck("plugin_id", &ids).Error; err != nil {
		return nil, fmt.Errorf("load disabled agent plugins: %w", err)
	}
	for _, id := range ids {
		disabled[id] = true
	}
	return disabled, nil
}

func applyGitHubPairRule(ctx context.Context, db *gorm.DB, agent model.Agent, set map[uuid.UUID]bool) error {
	if len(set) < 2 {
		return nil
	}
	ids := make([]uuid.UUID, 0, len(set))
	for id := range set {
		ids = append(ids, id)
	}
	var rows []model.PluginIntegration
	if err := db.WithContext(ctx).
		Model(&model.PluginIntegration{}).
		Where("plugin_id IN ? AND kind = ? AND provider IN ?",
			ids, model.PluginIntegrationKindIntegration, gitHubPairProviders).
		Find(&rows).Error; err != nil {
		return fmt.Errorf("load github pair integrations: %w", err)
	}
	var primaryID, reviewsID uuid.UUID
	for _, row := range rows {
		switch row.Provider {
		case "github-app":
			primaryID = row.PluginID
		case "github-app-code-reviews":
			reviewsID = row.PluginID
		}
	}
	if primaryID == uuid.Nil || reviewsID == uuid.Nil {
		return nil
	}

	required, err := agentCatalogRequiredPlugins(ctx, db, agent)
	if err != nil {
		return err
	}
	keepReviews := false
	if reviewsSlug, ok := pluginSlug(ctx, db, reviewsID); ok && required[reviewsSlug] {
		keepReviews = true
	}
	if keepReviews {
		delete(set, primaryID)
	} else {
		delete(set, reviewsID)
	}
	return nil
}

func agentCatalogRequiredPlugins(ctx context.Context, db *gorm.DB, agent model.Agent) (map[string]bool, error) {
	required := map[string]bool{}
	if agent.AgentCatalog != nil {
		for _, slug := range agent.AgentCatalog.RequiredPlugins {
			required[slug] = true
		}
		return required, nil
	}
	if agent.AgentCatalogID == nil {
		return required, nil
	}
	var catalog model.AgentCatalog
	if err := db.WithContext(ctx).
		Select("required_plugins").
		Where("id = ?", *agent.AgentCatalogID).
		First(&catalog).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return required, nil
		}
		return nil, fmt.Errorf("load agent catalog required plugins: %w", err)
	}
	for _, slug := range catalog.RequiredPlugins {
		required[slug] = true
	}
	return required, nil
}

func pluginSlug(ctx context.Context, db *gorm.DB, pluginID uuid.UUID) (string, bool) {
	var slug string
	if err := db.WithContext(ctx).
		Model(&model.Plugin{}).
		Where("id = ?", pluginID).
		Pluck("slug", &slug).Error; err != nil {
		return "", false
	}
	return slug, slug != ""
}
