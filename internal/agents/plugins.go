package agents

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/pluginresolve"
)

// ConnectionRequirement mirrors a plugin's unmet connection requirement for
// tool responses and validation errors.
type ConnectionRequirement struct {
	Provider string `json:"provider"`
	Kind     string `json:"kind"`
	Required bool   `json:"required"`
}

// resolvePluginSlugs validates that every requested slug is an active plugin
// available to the target team and returns the matching plugin rows. On an unknown or
// uninstalled slug it returns a helpful error listing the installed slugs and
// noting any missing requirements for the offending plugin when it exists but
// is not usable.
func resolvePluginSlugs(ctx context.Context, db *gorm.DB, orgID, teamID uuid.UUID, requested []string) ([]model.Plugin, error) {
	cleaned := dedupeNonEmpty(requested)
	if len(cleaned) == 0 {
		return nil, nil
	}
	installed, err := installedPluginsBySlug(ctx, db, orgID, teamID)
	if err != nil {
		return nil, err
	}
	installedSlugs := make([]string, 0, len(installed))
	for slug := range installed {
		installedSlugs = append(installedSlugs, slug)
	}
	sort.Strings(installedSlugs)

	out := make([]model.Plugin, 0, len(cleaned))
	for _, slug := range cleaned {
		plugin, ok := installed[slug]
		if !ok {
			return nil, unknownPluginError(ctx, db, orgID, teamID, slug, installedSlugs)
		}
		out = append(out, plugin)
	}
	return out, nil
}

func unknownPluginError(ctx context.Context, db *gorm.DB, orgID, teamID uuid.UUID, slug string, installedSlugs []string) error {
	// Distinguish "exists but not installed" from "does not exist" so the caller
	// gets actionable guidance, including missing connection requirements.
	var plugin model.Plugin
	err := db.WithContext(ctx).
		Where("slug = ? AND status = ? AND (org_id IS NULL OR (org_id = ? AND team_id = ?))", slug, model.PluginStatusActive, orgID, teamID).
		First(&plugin).Error
	if err == nil {
		missing, mErr := missingRequirements(ctx, db, orgID, plugin.ID)
		if mErr == nil && len(missing) > 0 {
			return fmt.Errorf(
				"plugin %q is not enabled for this team and has unmet requirements: %s (team plugins: %s)",
				slug, formatRequirements(missing), joinOrNone(installedSlugs),
			)
		}
		return fmt.Errorf(
			"plugin %q is not enabled for this team (team plugins: %s)",
			slug, joinOrNone(installedSlugs),
		)
	}
	return fmt.Errorf(
		"unknown plugin %q (installed plugins: %s)",
		slug, joinOrNone(installedSlugs),
	)
}

// installedPluginsBySlug returns active plugins effective for a regular agent
// on the team, keyed by slug.
func installedPluginsBySlug(ctx context.Context, db *gorm.DB, orgID, teamID uuid.UUID) (map[string]model.Plugin, error) {
	ids, err := pluginresolve.EffectivePluginIDs(ctx, db, model.Agent{OrgID: &orgID, TeamID: teamID})
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return map[string]model.Plugin{}, nil
	}
	var plugins []model.Plugin
	if err := db.WithContext(ctx).
		Where("id IN ? AND status = ? AND (org_id IS NULL OR (org_id = ? AND team_id = ?))", ids, model.PluginStatusActive, orgID, teamID).
		Find(&plugins).Error; err != nil {
		return nil, fmt.Errorf("load team plugins: %w", err)
	}
	out := make(map[string]model.Plugin, len(plugins))
	for _, plugin := range plugins {
		out[plugin.Slug] = plugin
	}
	return out, nil
}

// missingRequirements returns the plugin's required connections the org has not
// satisfied. Mirrors handler.PluginHandler.missingRequirements.
func missingRequirements(ctx context.Context, db *gorm.DB, orgID, pluginID uuid.UUID) ([]ConnectionRequirement, error) {
	var reqs []model.PluginIntegration
	if err := db.WithContext(ctx).
		Where("plugin_id = ? AND required = true", pluginID).
		Find(&reqs).Error; err != nil {
		return nil, err
	}
	missing := make([]ConnectionRequirement, 0)
	for _, req := range reqs {
		ok, err := hasConnectionRequirement(ctx, db, orgID, req)
		if err != nil {
			return nil, err
		}
		if !ok {
			missing = append(missing, ConnectionRequirement{Provider: req.Provider, Kind: req.Kind, Required: req.Required})
		}
	}
	return missing, nil
}

func hasConnectionRequirement(ctx context.Context, db *gorm.DB, orgID uuid.UUID, req model.PluginIntegration) (bool, error) {
	switch req.Kind {
	case model.PluginIntegrationKindDatabase:
		var count int64
		err := db.WithContext(ctx).Model(&model.DatabaseConnection{}).
			Where("org_id = ? AND provider = ? AND revoked_at IS NULL", orgID, req.Provider).
			Count(&count).Error
		return count > 0, err
	default:
		var count int64
		err := db.WithContext(ctx).Model(&model.Connection{}).
			Joins("JOIN integrations ON integrations.id = connections.integration_id AND integrations.deleted_at IS NULL").
			Where("connections.org_id = ? AND connections.revoked_at IS NULL AND integrations.provider = ?", orgID, req.Provider).
			Count(&count).Error
		return count > 0, err
	}
}

func formatRequirements(reqs []ConnectionRequirement) string {
	parts := make([]string, 0, len(reqs))
	for _, req := range reqs {
		parts = append(parts, fmt.Sprintf("%s (%s)", req.Provider, req.Kind))
	}
	return strings.Join(parts, ", ")
}

func joinOrNone(values []string) string {
	if len(values) == 0 {
		return "none"
	}
	return strings.Join(values, ", ")
}
