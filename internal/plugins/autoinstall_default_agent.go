package plugins

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/model"
)

// PluginDefaultAgentInstall reports whether the plugin should be installed only
// on each org's default (Hivy) agent, rather than on every agent like
// auto_install. Such plugins still appear in the catalog for opt-in install on
// other agents.
func PluginDefaultAgentInstall(plugin model.Plugin) bool {
	return manifestBool(plugin.Manifest, "default_agent_install")
}

func defaultAgentInstallPlugins(ctx context.Context, tx *gorm.DB) ([]model.Plugin, error) {
	var rows []model.Plugin
	if err := tx.WithContext(ctx).
		Where("status = ? AND org_id IS NULL", model.PluginStatusActive).
		Order("slug ASC").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("load default-agent plugins: %w", err)
	}
	out := make([]model.Plugin, 0, len(rows))
	for _, row := range rows {
		if PluginDefaultAgentInstall(row) {
			out = append(out, row)
		}
	}
	return out, nil
}

// EnsureDefaultAgentPluginsForAgent attaches every active "default_agent_install"
// plugin to the org and the given agent. Call this only for an org's default
// (Hivy) agent, at provisioning time.
func EnsureDefaultAgentPluginsForAgent(ctx context.Context, tx *gorm.DB, orgID, agentID uuid.UUID) error {
	if orgID == uuid.Nil || agentID == uuid.Nil {
		return nil
	}
	plugins, err := defaultAgentInstallPlugins(ctx, tx)
	if err != nil {
		return err
	}
	for _, plugin := range plugins {
		if err := ensureOrgPluginInstall(ctx, tx, orgID, plugin.ID); err != nil {
			return err
		}
		if err := ensureAgentPluginInstall(ctx, tx, orgID, agentID, plugin.ID); err != nil {
			return err
		}
		if err := refreshPluginSkillInstallCounts(ctx, tx, plugin.ID); err != nil {
			return err
		}
	}
	return nil
}

// ReconcileDefaultAgentInstalled backfills active "default_agent_install" plugins
// into every org (that has a default agent) and each org's default agent
// (agents.is_default = true). It never touches non-default agents.
func ReconcileDefaultAgentInstalled(ctx context.Context, tx *gorm.DB) error {
	plugins, err := defaultAgentInstallPlugins(ctx, tx)
	if err != nil {
		return err
	}
	for _, plugin := range plugins {
		if err := tx.WithContext(ctx).Exec(`
			INSERT INTO org_plugin_installs (id, org_id, plugin_id, created_at, updated_at)
			SELECT gen_random_uuid(), orgs.id, ?, NOW(), NOW()
			FROM orgs
			WHERE EXISTS (
				SELECT 1 FROM agents
				WHERE agents.org_id = orgs.id
					AND agents.is_default = true
					AND agents.status <> ?
			)
			AND NOT EXISTS (
				SELECT 1 FROM org_plugin_installs installs
				WHERE installs.org_id = orgs.id
					AND installs.plugin_id = ?
					AND installs.revoked_at IS NULL
			)
			FOR KEY SHARE OF orgs
			ON CONFLICT DO NOTHING
		`, plugin.ID, "archived", plugin.ID).Error; err != nil {
			return fmt.Errorf("backfill org default-agent plugin %q: %w", plugin.Slug, err)
		}
		if err := tx.WithContext(ctx).Exec(`
			INSERT INTO agent_plugin_installs (org_id, agent_id, plugin_id, created_at)
			SELECT agents.org_id, agents.id, ?, NOW()
			FROM agents
			WHERE agents.org_id IS NOT NULL
				AND agents.is_default = true
				AND agents.status <> ?
			FOR KEY SHARE OF agents
			ON CONFLICT DO NOTHING
		`, plugin.ID, "archived").Error; err != nil {
			return fmt.Errorf("backfill default-agent plugin %q: %w", plugin.Slug, err)
		}
		if err := refreshPluginSkillInstallCounts(ctx, tx, plugin.ID); err != nil {
			return err
		}
	}
	return nil
}

func ensureOrgPluginInstall(ctx context.Context, tx *gorm.DB, orgID, pluginID uuid.UUID) error {
	install := model.OrgPluginInstall{
		ID:       uuid.New(),
		OrgID:    orgID,
		PluginID: pluginID,
	}
	if err := tx.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&install).Error; err != nil {
		return fmt.Errorf("ensure org plugin install: %w", err)
	}
	return nil
}

func ensureAgentPluginInstall(ctx context.Context, tx *gorm.DB, orgID, agentID, pluginID uuid.UUID) error {
	install := model.AgentPluginInstall{OrgID: orgID, AgentID: agentID, PluginID: pluginID}
	if err := tx.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&install).Error; err != nil {
		return fmt.Errorf("ensure agent plugin install: %w", err)
	}
	return nil
}

func refreshPluginSkillInstallCounts(ctx context.Context, tx *gorm.DB, pluginID uuid.UUID) error {
	return tx.WithContext(ctx).Model(&model.Skill{}).
		Where("plugin_id = ?", pluginID).
		UpdateColumn("install_count", gorm.Expr(
			"(SELECT COUNT(*) FROM agent_plugin_installs WHERE plugin_id = ?)", pluginID,
		)).Error
}
