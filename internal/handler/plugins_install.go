package handler

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	pluginstore "github.com/usehivy/hivy/internal/plugins"
)

// disablePluginForOrg drops the plugin's team grants across the org when the org
// uninstalls it, then refreshes the plugin's skill install counts. The resolver's
// intersection with active org installs already makes stale grants inert; the
// delete keeps team_plugins clean.
func disablePluginForOrg(ctx context.Context, tx *gorm.DB, orgID, pluginID uuid.UUID) error {
	if err := tx.WithContext(ctx).
		Where("org_id = ? AND plugin_id = ?", orgID, pluginID).
		Delete(&model.TeamPlugin{}).Error; err != nil {
		return err
	}
	return pluginstore.RefreshPluginSkillInstallCounts(ctx, tx, pluginID)
}

// pluginRequiredByOrgAgents reports whether any non-archived agent in the org has
// the plugin's slug among its catalog's required plugins.
func pluginRequiredByOrgAgents(ctx context.Context, db *gorm.DB, orgID uuid.UUID, slug string) (bool, error) {
	if slug == "" {
		return false, nil
	}
	var count int64
	if err := db.WithContext(ctx).Model(&model.Agent{}).
		Joins("JOIN agent_catalog ON agent_catalog.id = agents.agent_catalog_id").
		Where("agents.org_id = ? AND agents.status <> ?", orgID, "archived").
		Where("? = ANY(agent_catalog.required_plugins)", slug).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}
