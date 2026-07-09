// Package teamprovision is the domain layer for team-primary provisioning:
// which plugins are enabled for a team and which knowledge (RAG) sources are
// granted to it. Teams are the provisioning unit — an agent inherits its
// capabilities from the team it is assigned to (team primary-authz model). All
// mutations here are org-admin actions; the HTTP layer applies the admin gate.
//
// The two backing tables are allowlists:
//   - team_plugins:     (org_id, team_id, plugin_id) — the team's plugin set.
//   - team_rag_sources: (org_id, team_id, rag_source_id) — the team's KB set.
package teamprovision

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

// Sentinel errors returned by the mutating helpers. Callers map these to HTTP
// statuses (404 for the *NotFound errors, 422 for ErrPluginNotInstalled).
var (
	// ErrTeamNotFound means the team does not exist in the given org (or is
	// archived). Guards every mutation so a caller cannot provision a team in
	// another org.
	ErrTeamNotFound = errors.New("teamprovision: team not found in org")
	// ErrPluginNotFound means the plugin is neither a global catalog plugin nor
	// owned by the given org.
	ErrPluginNotFound = errors.New("teamprovision: plugin not found in org")
	// ErrPluginNotInstalled means the plugin exists but the org has not
	// installed it (no active org_plugin_installs row). A team may only enable
	// plugins the org has installed.
	ErrPluginNotInstalled = errors.New("teamprovision: plugin not installed for org")
	// ErrPluginAlwaysEnabled means the plugin is an auto-install system plugin
	// (e.g. sheets, service-discovery, skill-manager, runtime). These are
	// implicitly enabled for every team and are not per-team toggleable, so
	// enabling or disabling them via team provisioning is rejected. Callers map
	// this to 422.
	ErrPluginAlwaysEnabled = errors.New("teamprovision: plugin is always enabled")
	// ErrSourceNotFound means the RAG source does not exist in the given org.
	ErrSourceNotFound = errors.New("teamprovision: rag source not found in org")
	// ErrPluginRequiredByAgents means a non-archived agent on the team requires
	// the plugin via its catalog's required_plugins. Disabling it would strip a
	// plugin an active catalog agent depends on, so the toggle is refused. Callers
	// map this to 409.
	ErrPluginRequiredByAgents = errors.New("teamprovision: plugin is required by active team agents")
)

// PluginEnabledForTeam reports whether a team_plugins row exists for the pair.
func PluginEnabledForTeam(ctx context.Context, db *gorm.DB, teamID, pluginID uuid.UUID) (bool, error) {
	var count int64
	err := db.WithContext(ctx).Model(&model.TeamPlugin{}).
		Where("team_id = ? AND plugin_id = ?", teamID, pluginID).
		Count(&count).Error
	return count > 0, err
}

// SourceGrantedToTeam reports whether a team_rag_sources row exists for the pair.
func SourceGrantedToTeam(ctx context.Context, db *gorm.DB, teamID, ragSourceID uuid.UUID) (bool, error) {
	var count int64
	err := db.WithContext(ctx).Model(&model.TeamRagSource{}).
		Where("team_id = ? AND rag_source_id = ?", teamID, ragSourceID).
		Count(&count).Error
	return count > 0, err
}

// EnabledPluginIDs returns the plugin ids enabled for a team, name-agnostic and
// unordered from the DB's perspective (callers order for display).
func EnabledPluginIDs(ctx context.Context, db *gorm.DB, teamID uuid.UUID) ([]uuid.UUID, error) {
	ids := []uuid.UUID{}
	err := db.WithContext(ctx).Model(&model.TeamPlugin{}).
		Where("team_id = ?", teamID).
		Pluck("plugin_id", &ids).Error
	return ids, err
}

// GrantedSourceIDs returns the rag source ids granted to a team.
func GrantedSourceIDs(ctx context.Context, db *gorm.DB, teamID uuid.UUID) ([]uuid.UUID, error) {
	ids := []uuid.UUID{}
	err := db.WithContext(ctx).Model(&model.TeamRagSource{}).
		Where("team_id = ?", teamID).
		Pluck("rag_source_id", &ids).Error
	return ids, err
}

// teamInOrg reports whether teamID names an active team in orgID. Used by every
// mutation for org-scoping defense in depth.
func teamInOrg(ctx context.Context, db *gorm.DB, orgID, teamID uuid.UUID) (bool, error) {
	var count int64
	err := db.WithContext(ctx).Model(&model.Team{}).
		Where("id = ? AND org_id = ? AND archived_at IS NULL", teamID, orgID).
		Count(&count).Error
	return count > 0, err
}
