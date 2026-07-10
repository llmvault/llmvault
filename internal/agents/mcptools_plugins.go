package agents

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

// --- list_org_plugins --------------------------------------------------------

func registerListOrgPlugins(server *mcp.Server, db *gorm.DB, token *model.Token, frontendURL string) {
	server.AddTool(&mcp.Tool{
		Name:        toolListOrgPlugins,
		Description: "List the plugins available to this organization, split into installed and available. Each plugin lists its skills, required connections, and an install_url (the page to send the user to install/connect it); available plugins also list missing_requirements. Plugins are enabled at the team level and inherited by default; a user may disable an optional inherited plugin for one agent in Agent details. Use this to discover which skills you can pass to create_agent / update_agent.",
		InputSchema: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties":           map[string]any{},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handleListOrgPlugins(ctx, db, token, frontendURL)
	})
}

func handleListOrgPlugins(ctx context.Context, db *gorm.DB, token *model.Token, frontendURL string) (*mcp.CallToolResult, error) {
	var plugins []model.Plugin
	if err := db.WithContext(ctx).
		Where("status = ? AND (org_id IS NULL OR org_id = ?)", model.PluginStatusActive, token.OrgID).
		Order("name ASC").Find(&plugins).Error; err != nil {
		return toolError("failed to list plugins: " + err.Error()), nil
	}
	installedIDs, err := installedPluginIDSet(ctx, db, token.OrgID)
	if err != nil {
		return toolError(err.Error()), nil
	}
	installed := make([]map[string]any, 0)
	available := make([]map[string]any, 0)
	for _, plugin := range plugins {
		obj, err := pluginObject(ctx, db, token.OrgID, plugin, !installedIDs[plugin.ID], frontendURL)
		if err != nil {
			return toolError(err.Error()), nil
		}
		if installedIDs[plugin.ID] {
			installed = append(installed, obj)
		} else {
			available = append(available, obj)
		}
	}
	return toolJSON(map[string]any{
		"installed": installed,
		"available": available,
	})
}

func installedPluginIDSet(ctx context.Context, db *gorm.DB, orgID uuid.UUID) (map[uuid.UUID]bool, error) {
	var pluginIDs []uuid.UUID
	if err := db.WithContext(ctx).Model(&model.OrgPluginInstall{}).
		Where("org_id = ? AND revoked_at IS NULL", orgID).
		Distinct("plugin_id").
		Pluck("plugin_id", &pluginIDs).Error; err != nil {
		return nil, fmt.Errorf("load org plugin installs: %w", err)
	}
	out := make(map[uuid.UUID]bool, len(pluginIDs))
	for _, id := range pluginIDs {
		out[id] = true
	}
	return out, nil
}

func pluginObject(ctx context.Context, db *gorm.DB, orgID uuid.UUID, plugin model.Plugin, includeMissing bool, frontendURL string) (map[string]any, error) {
	var skills []model.Skill
	if err := db.WithContext(ctx).
		Where("plugin_id = ? AND status = ?", plugin.ID, model.SkillStatusPublished).
		Order("name ASC").
		Find(&skills).Error; err != nil {
		return nil, fmt.Errorf("load plugin skills: %w", err)
	}
	skillObjs := make([]map[string]any, 0, len(skills))
	for _, skill := range skills {
		desc := ""
		if skill.Description != nil {
			desc = *skill.Description
		}
		skillObjs = append(skillObjs, map[string]any{
			"slug":        skill.Slug,
			"name":        skill.Name,
			"description": desc,
		})
	}
	var reqs []model.PluginIntegration
	if err := db.WithContext(ctx).Where("plugin_id = ?", plugin.ID).Order("provider ASC").Find(&reqs).Error; err != nil {
		return nil, fmt.Errorf("load plugin requirements: %w", err)
	}
	reqObjs := make([]map[string]any, 0, len(reqs))
	for _, req := range reqs {
		reqObjs = append(reqObjs, map[string]any{
			"provider": req.Provider,
			"kind":     req.Kind,
			"required": req.Required,
		})
	}
	obj := map[string]any{
		"id":                   plugin.ID.String(),
		"slug":                 plugin.Slug,
		"name":                 plugin.Name,
		"description":          plugin.Description,
		"category":             plugin.Category,
		"skills":               skillObjs,
		"required_connections": reqObjs,
		"install_url":          pluginInstallURL(frontendURL, plugin.Slug),
	}
	if includeMissing {
		missing, err := missingRequirements(ctx, db, orgID, plugin.ID)
		if err != nil {
			return nil, fmt.Errorf("load missing requirements: %w", err)
		}
		missingObjs := make([]map[string]any, 0, len(missing))
		for _, req := range missing {
			missingObjs = append(missingObjs, map[string]any{
				"provider": req.Provider,
				"kind":     req.Kind,
				"required": req.Required,
			})
		}
		obj["missing_requirements"] = missingObjs
	}
	return obj, nil
}

// pluginInstallURL is the app page where a user installs/connects a plugin.
func pluginInstallURL(frontendURL, slug string) string {
	base := strings.TrimRight(strings.TrimSpace(frontendURL), "/")
	return base + "/w/plugins/" + slug
}
