package handler

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/model"
)

func enablePluginForAgent(ctx context.Context, tx *gorm.DB, orgID, agentID, pluginID uuid.UUID) error {
	install := model.AgentPluginInstall{OrgID: orgID, AgentID: agentID, PluginID: pluginID}
	if err := tx.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&install).Error; err != nil {
		return fmt.Errorf("enable plugin for agent: %w", err)
	}
	// Skills are inherited dynamically from the plugin at runtime, so there is
	// nothing to materialize here beyond the plugin install itself.
	return refreshPluginSkillInstallCounts(ctx, tx, pluginID)
}

func disablePluginForOrg(ctx context.Context, tx *gorm.DB, orgID, pluginID uuid.UUID) error {
	var installs []model.AgentPluginInstall
	if err := tx.WithContext(ctx).Where("org_id = ? AND plugin_id = ?", orgID, pluginID).Find(&installs).Error; err != nil {
		return err
	}
	for _, install := range installs {
		if err := disablePluginForAgent(ctx, tx, orgID, install.AgentID, pluginID); err != nil {
			return err
		}
	}
	return nil
}

func disablePluginForAgent(ctx context.Context, tx *gorm.DB, orgID, agentID, pluginID uuid.UUID) error {
	if err := tx.WithContext(ctx).
		Where("org_id = ? AND agent_id = ? AND plugin_id = ?", orgID, agentID, pluginID).
		Delete(&model.AgentPluginInstall{}).Error; err != nil {
		return err
	}
	return refreshPluginSkillInstallCounts(ctx, tx, pluginID)
}

// refreshPluginSkillInstallCounts sets each of the plugin's skills' install_count
// to the number of agents that have the plugin installed (skills are inherited
// through plugin installs, so per-skill counts mirror the plugin's reach).
func refreshPluginSkillInstallCounts(ctx context.Context, tx *gorm.DB, pluginID uuid.UUID) error {
	return tx.WithContext(ctx).Model(&model.Skill{}).
		Where("plugin_id = ?", pluginID).
		UpdateColumn("install_count", gorm.Expr(
			"(SELECT COUNT(*) FROM agent_plugin_installs WHERE plugin_id = ?)", pluginID,
		)).Error
}
