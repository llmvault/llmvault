package pluginresolve

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

// RefreshPluginSkillInstallCounts sets each of the plugin's skills' install_count
// to the number of non-archived agents whose effective plugin set contains the
// plugin: every agent for auto-install plugins, otherwise agents in granting
// teams (of orgs with an active install) minus per-agent disabled overrides,
// plus default agents for default-agent-install plugins. The count is derived
// with a single set-based query rather than a per-agent fan-out, which
// deadlocked under concurrent load.
func RefreshPluginSkillInstallCounts(ctx context.Context, tx *gorm.DB, pluginID uuid.UUID) error {
	count, err := effectiveAgentCountForPlugin(ctx, tx, pluginID)
	if err != nil {
		return err
	}
	return tx.WithContext(ctx).Model(&model.Skill{}).
		Where("plugin_id = ?", pluginID).
		UpdateColumn("install_count", count).Error
}

func effectiveAgentCountForPlugin(ctx context.Context, tx *gorm.DB, pluginID uuid.UUID) (int64, error) {
	if pluginID == uuid.Nil {
		return 0, nil
	}
	var plugin model.Plugin
	if err := tx.WithContext(ctx).Where("id = ?", pluginID).First(&plugin).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return 0, nil
		}
		return 0, fmt.Errorf("load plugin for count: %w", err)
	}
	var count int64
	if PluginAutoInstall(plugin) {
		if err := tx.WithContext(ctx).Model(&model.Agent{}).
			Where("org_id IS NOT NULL AND status <> ?", "archived").
			Count(&count).Error; err != nil {
			return 0, fmt.Errorf("count agents for auto-install plugin: %w", err)
		}
		return count, nil
	}
	if err := tx.WithContext(ctx).
		Table("agents").
		Joins("JOIN org_plugin_installs opi ON opi.org_id = agents.org_id AND opi.plugin_id = ? AND opi.revoked_at IS NULL", pluginID).
		Joins("LEFT JOIN team_plugins tp ON tp.team_id = agents.team_id AND tp.plugin_id = ?", pluginID).
		Joins("LEFT JOIN agent_plugin_overrides apo ON apo.agent_id = agents.id AND apo.plugin_id = ?", pluginID).
		Where("agents.status <> ?", "archived").
		Where("((tp.plugin_id IS NOT NULL AND apo.plugin_id IS NULL) OR (? AND agents.is_default))", PluginDefaultAgentInstall(plugin)).
		Distinct("agents.id").
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("count agents for plugin: %w", err)
	}
	return count, nil
}
