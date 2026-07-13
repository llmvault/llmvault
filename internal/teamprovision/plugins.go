package teamprovision

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/plugins"
)

// EnablePlugin adds pluginID to teamID's plugin allowlist. It is idempotent:
// enabling an already-enabled plugin is a no-op success. It validates that the
// team belongs to orgID, that the plugin is visible to the team (global or
// owned by that exact team), and that the org has installed the plugin.
// enabledBy records the acting user (nil for API-key callers).
//
// Errors: ErrTeamNotFound, ErrPluginNotFound, ErrPluginNotInstalled.
func EnablePlugin(ctx context.Context, db *gorm.DB, orgID, teamID, pluginID uuid.UUID, enabledBy *uuid.UUID) error {
	ok, err := teamInOrg(ctx, db, orgID, teamID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrTeamNotFound
	}
	if err := validatePluginForTeam(ctx, db, orgID, teamID, pluginID); err != nil {
		return err
	}
	// Auto-install system plugins are always enabled for every team, so there is
	// no team_plugins row to create — reject the toggle instead of writing a
	// meaningless grant.
	if always, err := pluginAlwaysEnabled(ctx, db, pluginID); err != nil {
		return err
	} else if always {
		return ErrPluginAlwaysEnabled
	}
	row := model.TeamPlugin{
		OrgID:     orgID,
		TeamID:    teamID,
		PluginID:  pluginID,
		EnabledBy: enabledBy,
	}
	// DoNothing keeps the original enabled_by/created_at on a repeat enable.
	if err := db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "team_id"}, {Name: "plugin_id"}},
		DoNothing: true,
	}).Create(&row).Error; err != nil {
		return err
	}
	return plugins.RefreshPluginSkillInstallCounts(ctx, db, pluginID)
}

// DisablePlugin removes pluginID from teamID's plugin allowlist. Idempotent:
// disabling a plugin that is not enabled is a no-op success. It still validates
// the team belongs to orgID so a caller cannot probe other orgs.
func DisablePlugin(ctx context.Context, db *gorm.DB, orgID, teamID, pluginID uuid.UUID) error {
	ok, err := teamInOrg(ctx, db, orgID, teamID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrTeamNotFound
	}
	// Auto-install system plugins are always enabled and cannot be turned off per
	// team; reject rather than silently no-op so the caller sees the reason.
	if always, err := pluginAlwaysEnabled(ctx, db, pluginID); err != nil {
		return err
	} else if always {
		return ErrPluginAlwaysEnabled
	}
	// A team admin cannot strip a plugin that an active catalog agent on the team
	// requires — its effective set would lose a plugin its catalog depends on.
	if required, err := pluginRequiredByTeamAgents(ctx, db, teamID, pluginID); err != nil {
		return err
	} else if required {
		return ErrPluginRequiredByAgents
	}
	if err := db.WithContext(ctx).
		Where("org_id = ? AND team_id = ? AND plugin_id = ?", orgID, teamID, pluginID).
		Delete(&model.TeamPlugin{}).Error; err != nil {
		return err
	}
	return plugins.RefreshPluginSkillInstallCounts(ctx, db, pluginID)
}

// pluginRequiredByTeamAgents reports whether any non-archived agent on the team
// has the plugin's slug among its catalog's required plugins.
func pluginRequiredByTeamAgents(ctx context.Context, db *gorm.DB, teamID, pluginID uuid.UUID) (bool, error) {
	var slug string
	if err := db.WithContext(ctx).Model(&model.Plugin{}).
		Where("id = ?", pluginID).
		Pluck("slug", &slug).Error; err != nil {
		return false, err
	}
	if slug == "" {
		return false, nil
	}
	var count int64
	if err := db.WithContext(ctx).Model(&model.Agent{}).
		Joins("JOIN agent_catalog ON agent_catalog.id = agents.agent_catalog_id").
		Where("agents.team_id = ? AND agents.status <> ?", teamID, "archived").
		Where("? = ANY(agent_catalog.required_plugins)", slug).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// pluginAlwaysEnabled reports whether pluginID names an auto-install system
// plugin. Such plugins are implicitly enabled for every team and are excluded
// from per-team provisioning. A missing plugin is treated as not always-enabled
// (the caller's validatePluginForOrg surfaces the not-found error).
func pluginAlwaysEnabled(ctx context.Context, db *gorm.DB, pluginID uuid.UUID) (bool, error) {
	var plugin model.Plugin
	if err := db.WithContext(ctx).Where("id = ?", pluginID).First(&plugin).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	return plugins.PluginAutoInstall(plugin), nil
}

// validatePluginForTeam checks the plugin is visible to the team (global or
// owned by that exact team) and installed by the org. A plugin owned by another
// team is indistinguishable from a nonexistent plugin.
func validatePluginForTeam(ctx context.Context, db *gorm.DB, orgID, teamID, pluginID uuid.UUID) error {
	var visible int64
	if err := db.WithContext(ctx).Model(&model.Plugin{}).
		Where("id = ? AND (org_id IS NULL OR (org_id = ? AND team_id = ?))", pluginID, orgID, teamID).
		Count(&visible).Error; err != nil {
		return err
	}
	if visible == 0 {
		return ErrPluginNotFound
	}
	var installed int64
	if err := db.WithContext(ctx).Model(&model.OrgPluginInstall{}).
		Where("org_id = ? AND plugin_id = ? AND revoked_at IS NULL", orgID, pluginID).
		Count(&installed).Error; err != nil {
		return err
	}
	if installed == 0 {
		return ErrPluginNotInstalled
	}
	return nil
}
