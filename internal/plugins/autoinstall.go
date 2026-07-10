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

// PluginAutoInstall reports whether the plugin should be installed into every
// org automatically and resolved onto every agent.
func PluginAutoInstall(plugin model.Plugin) bool {
	return pluginresolve.PluginAutoInstall(plugin)
}

// PluginLocked reports whether users may remove the plugin. Auto-installed
// plugins are always locked, even if the manifest omits "locked".
func PluginLocked(plugin model.Plugin) bool {
	return PluginAutoInstall(plugin) || pluginresolve.ManifestBool(plugin.Manifest, "locked")
}

func autoInstallPlugins(ctx context.Context, tx *gorm.DB) ([]model.Plugin, error) {
	var rows []model.Plugin
	if err := tx.WithContext(ctx).
		Clauses(clause.Locking{Strength: "KEY SHARE"}).
		Where("status = ? AND org_id IS NULL", model.PluginStatusActive).
		Order("slug ASC").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("load auto-install plugins: %w", err)
	}
	out := make([]model.Plugin, 0, len(rows))
	for _, row := range rows {
		if PluginAutoInstall(row) {
			out = append(out, row)
		}
	}
	return out, nil
}

// EnsureAutoInstalledForOrg installs every active global auto-install plugin at
// the org level and refreshes each plugin's skill install counts. Agents resolve
// these plugins from the org install at runtime, so there is no per-agent work.
func EnsureAutoInstalledForOrg(ctx context.Context, tx *gorm.DB, orgID uuid.UUID) error {
	if orgID == uuid.Nil {
		return nil
	}
	plugins, err := autoInstallPlugins(ctx, tx)
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

// ReconcileAutoInstalled backfills active global auto-install plugins into every
// org's install set and refreshes skill install counts. Agents resolve these
// plugins from the org install, so nothing is written per agent.
func ReconcileAutoInstalled(ctx context.Context, tx *gorm.DB) error {
	plugins, err := autoInstallPlugins(ctx, tx)
	if err != nil {
		return err
	}
	for _, plugin := range plugins {
		if err := tx.WithContext(ctx).Exec(`
			INSERT INTO org_plugin_installs (id, org_id, plugin_id, created_at, updated_at)
			SELECT gen_random_uuid(), orgs.id, ?, NOW(), NOW()
			FROM orgs
			WHERE NOT EXISTS (
				SELECT 1
				FROM org_plugin_installs installs
				WHERE installs.org_id = orgs.id
					AND installs.plugin_id = ?
					AND installs.revoked_at IS NULL
			)
			FOR KEY SHARE OF orgs
			ON CONFLICT DO NOTHING
		`, plugin.ID, plugin.ID).Error; err != nil {
			return fmt.Errorf("backfill org auto-install plugin %q: %w", plugin.Slug, err)
		}
		if err := RefreshPluginSkillInstallCounts(ctx, tx, plugin.ID); err != nil {
			return err
		}
	}
	return reconcileCatalogRequiredPlugins(ctx, tx)
}

// reconcileCatalogRequiredPlugins refreshes skill install counts for every
// catalog-required plugin. Grants live on teams now, so there are no agent-level
// rows to backfill — only counts to keep current.
func reconcileCatalogRequiredPlugins(ctx context.Context, tx *gorm.DB) error {
	var pluginIDs []uuid.UUID
	if err := tx.WithContext(ctx).Table("plugins").
		Joins("JOIN agent_catalog catalog ON plugins.slug = ANY(catalog.required_plugins)").
		Where("plugins.status = ? AND plugins.org_id IS NULL", model.PluginStatusActive).
		Distinct("plugins.id").
		Pluck("plugins.id", &pluginIDs).Error; err != nil {
		return fmt.Errorf("list catalog-required plugins for count refresh: %w", err)
	}
	for _, pluginID := range pluginIDs {
		if err := RefreshPluginSkillInstallCounts(ctx, tx, pluginID); err != nil {
			return err
		}
	}
	return nil
}
