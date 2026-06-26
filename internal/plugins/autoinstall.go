package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/model"
)

// PluginAutoInstall reports whether the plugin should be installed into every
// org and every active agent automatically.
func PluginAutoInstall(plugin model.Plugin) bool {
	return manifestBool(plugin.Manifest, "auto_install")
}

// PluginLocked reports whether users may remove the plugin. Auto-installed
// plugins are always locked, even if the manifest omits "locked".
func PluginLocked(plugin model.Plugin) bool {
	return PluginAutoInstall(plugin) || manifestBool(plugin.Manifest, "locked")
}

func manifestBool(raw model.RawJSON, key string) bool {
	if len(raw) == 0 {
		return false
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return false
	}
	value, ok := payload[key]
	if !ok {
		return false
	}
	switch v := value.(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(strings.TrimSpace(v), "true")
	default:
		return false
	}
}

func autoInstallPlugins(ctx context.Context, tx *gorm.DB) ([]model.Plugin, error) {
	var rows []model.Plugin
	if err := tx.WithContext(ctx).
		Where("status = ?", model.PluginStatusActive).
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

// EnsureAutoInstalledForAgent attaches every active global auto-install plugin
// to the org and the agent.
func EnsureAutoInstalledForAgent(ctx context.Context, tx *gorm.DB, orgID, agentID uuid.UUID) error {
	if orgID == uuid.Nil || agentID == uuid.Nil {
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
		if err := ensureAgentPluginInstall(ctx, tx, orgID, agentID, plugin.ID); err != nil {
			return err
		}
		if err := refreshPluginSkillInstallCounts(ctx, tx, plugin.ID); err != nil {
			return err
		}
	}
	return nil
}

// EnsureInstalledPluginForEligibleAgents enables an org-installed plugin on
// agents that should automatically receive it: global auto-install plugins go
// to every active agent, and catalog-required plugins go to installed catalog
// agents whose catalog marks the plugin as required.
func EnsureInstalledPluginForEligibleAgents(ctx context.Context, tx *gorm.DB, orgID uuid.UUID, plugin model.Plugin) error {
	if orgID == uuid.Nil || plugin.ID == uuid.Nil {
		return nil
	}
	if PluginAutoInstall(plugin) {
		if err := enableAutoInstallPluginForOrgAgents(ctx, tx, orgID, plugin.ID); err != nil {
			return err
		}
	}
	if err := enableCatalogRequiredPluginForOrgAgents(ctx, tx, orgID, plugin.ID, plugin.Slug); err != nil {
		return err
	}
	return refreshPluginSkillInstallCounts(ctx, tx, plugin.ID)
}

// ReconcileAutoInstalled backfills active global auto-install plugins into all
// orgs and active agents. It also restores agent-level installs for
// catalog-required plugins when the org already has the required plugin.
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
		if err := tx.WithContext(ctx).Exec(`
			INSERT INTO agent_plugin_installs (org_id, agent_id, plugin_id, created_at)
			SELECT agents.org_id, agents.id, ?, NOW()
			FROM agents
			JOIN orgs ON orgs.id = agents.org_id
			WHERE agents.org_id IS NOT NULL
				AND agents.status <> ?
			FOR KEY SHARE OF agents, orgs
			ON CONFLICT DO NOTHING
		`, plugin.ID, "archived").Error; err != nil {
			return fmt.Errorf("backfill agent auto-install plugin %q: %w", plugin.Slug, err)
		}
		if err := refreshPluginSkillInstallCounts(ctx, tx, plugin.ID); err != nil {
			return err
		}
	}
	return reconcileCatalogRequiredPlugins(ctx, tx)
}

func enableAutoInstallPluginForOrgAgents(ctx context.Context, tx *gorm.DB, orgID, pluginID uuid.UUID) error {
	return tx.WithContext(ctx).Exec(`
		INSERT INTO agent_plugin_installs (org_id, agent_id, plugin_id, created_at)
		SELECT agents.org_id, agents.id, ?, NOW()
		FROM agents
		WHERE agents.org_id = ?
			AND agents.status <> ?
		FOR KEY SHARE OF agents
		ON CONFLICT DO NOTHING
	`, pluginID, orgID, "archived").Error
}

func enableCatalogRequiredPluginForOrgAgents(ctx context.Context, tx *gorm.DB, orgID, pluginID uuid.UUID, slug string) error {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return nil
	}
	return tx.WithContext(ctx).Exec(`
		INSERT INTO agent_plugin_installs (org_id, agent_id, plugin_id, created_at)
		SELECT agents.org_id, agents.id, ?, NOW()
		FROM agents
		JOIN agent_catalog catalog ON catalog.id = agents.agent_catalog_id
		WHERE agents.org_id = ?
			AND agents.status <> ?
			AND ? = ANY(catalog.required_plugins)
		FOR KEY SHARE OF agents
		ON CONFLICT DO NOTHING
	`, pluginID, orgID, "archived", slug).Error
}

func reconcileCatalogRequiredPlugins(ctx context.Context, tx *gorm.DB) error {
	if err := tx.WithContext(ctx).Exec(`
		INSERT INTO agent_plugin_installs (org_id, agent_id, plugin_id, created_at)
		SELECT agents.org_id, agents.id, plugins.id, NOW()
		FROM agents
		JOIN agent_catalog catalog ON catalog.id = agents.agent_catalog_id
		JOIN plugins ON plugins.status = ? AND plugins.slug = ANY(catalog.required_plugins)
		JOIN org_plugin_installs installs
			ON installs.org_id = agents.org_id
			AND installs.plugin_id = plugins.id
			AND installs.revoked_at IS NULL
		WHERE agents.org_id IS NOT NULL
			AND agents.status <> ?
		FOR KEY SHARE OF agents
		ON CONFLICT DO NOTHING
	`, model.PluginStatusActive, "archived").Error; err != nil {
		return fmt.Errorf("backfill catalog-required agent plugins: %w", err)
	}

	var pluginIDs []uuid.UUID
	if err := tx.WithContext(ctx).Table("plugins").
		Joins("JOIN agent_catalog catalog ON plugins.slug = ANY(catalog.required_plugins)").
		Where("plugins.status = ?", model.PluginStatusActive).
		Distinct("plugins.id").
		Pluck("plugins.id", &pluginIDs).Error; err != nil {
		return fmt.Errorf("list catalog-required plugins for count refresh: %w", err)
	}
	for _, pluginID := range pluginIDs {
		if err := refreshPluginSkillInstallCounts(ctx, tx, pluginID); err != nil {
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
