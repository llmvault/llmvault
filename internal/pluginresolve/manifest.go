package pluginresolve

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

// PluginAutoInstall reports whether the plugin should be installed into every
// org automatically and resolved onto every agent.
func PluginAutoInstall(plugin model.Plugin) bool {
	return ManifestBool(plugin.Manifest, "auto_install")
}

// PluginDefaultAgentInstall reports whether the plugin should be installed only
// on each org's default (Hivy) agent, rather than on every agent like
// auto_install.
func PluginDefaultAgentInstall(plugin model.Plugin) bool {
	return ManifestBool(plugin.Manifest, "default_agent_install")
}

// ManifestBool reads a boolean-valued manifest key, treating a "true" string as
// true.
func ManifestBool(raw model.RawJSON, key string) bool {
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

func autoInstallPlugins(ctx context.Context, db *gorm.DB) ([]model.Plugin, error) {
	return manifestPlugins(ctx, db, "auto_install")
}

func defaultAgentInstallPlugins(ctx context.Context, db *gorm.DB) ([]model.Plugin, error) {
	return manifestPlugins(ctx, db, "default_agent_install")
}

func manifestPlugins(ctx context.Context, db *gorm.DB, key string) ([]model.Plugin, error) {
	var rows []model.Plugin
	if err := db.WithContext(ctx).
		Where("status = ? AND org_id IS NULL", model.PluginStatusActive).
		Order("slug ASC").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("load %s plugins: %w", key, err)
	}
	out := make([]model.Plugin, 0, len(rows))
	for _, row := range rows {
		if ManifestBool(row.Manifest, key) {
			out = append(out, row)
		}
	}
	return out, nil
}
