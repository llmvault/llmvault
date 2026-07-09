package plugins

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/pluginresolve"
)

// PluginDefaultAgentInstall reports whether the plugin should be installed only
// on each org's default (Hivy) agent, rather than on every agent like
// auto_install. Such plugins still appear in the catalog for opt-in install on
// other agents.
func PluginDefaultAgentInstall(plugin model.Plugin) bool {
	return pluginresolve.PluginDefaultAgentInstall(plugin)
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

// EnsureDefaultAgentPluginsForOrg installs every active "default_agent_install"
// plugin at the org level and refreshes skill install counts. The org's default
// (Hivy) agent resolves these plugins from the org install; other agents do not.
func EnsureDefaultAgentPluginsForOrg(ctx context.Context, tx *gorm.DB, orgID uuid.UUID) error {
	if orgID == uuid.Nil {
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
		if err := RefreshPluginSkillInstallCounts(ctx, tx, plugin.ID); err != nil {
			return err
		}
	}
	return nil
}

// ReconcileDefaultAgentInstalled backfills active "default_agent_install" plugins
// into the install set of every org that has a default agent, and refreshes skill
// install counts. The default agent resolves these plugins from the org install.
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
		if err := RefreshPluginSkillInstallCounts(ctx, tx, plugin.ID); err != nil {
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

// RefreshPluginSkillInstallCounts sets each of the plugin's skills' install_count
// to the number of non-archived agents whose effective plugin set contains the
// plugin.
func RefreshPluginSkillInstallCounts(ctx context.Context, tx *gorm.DB, pluginID uuid.UUID) error {
	return pluginresolve.RefreshPluginSkillInstallCounts(ctx, tx, pluginID)
}
