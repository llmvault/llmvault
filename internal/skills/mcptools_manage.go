package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/model"
)

// Tool names for the skill-manager group. All four are privileged, opt-in
// mutating tools gated per calling agent (see skillManagerEnabled).
const (
	toolCreateOrgPlugin = "create_org_plugin"
	toolCreateSkill     = "create_skill"
	toolUpdateSkill     = "update_skill"
	toolArchiveSkill    = "archive_skill"
)

// Limits for agent-authored skills. Generous enough for real skills with
// references and scripts, small enough that a runaway tool call cannot bloat
// the skills table.
const (
	maxSkillContentBytes   = 256 * 1024
	maxSkillFileBytes      = 256 * 1024
	maxSkillTotalBytes     = 1024 * 1024
	maxSkillFiles          = 32
	maxSkillNameLen        = 120
	maxSkillDescriptionLen = 1024
)

var envVarNamePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

// skillManagerEnabled reports whether the calling agent may use the privileged
// skill-manager tools. The default Hivy agent gets them automatically. Any
// other agent must explicitly allow-list one of them in McpToolFilter.Allow.
// Mirrors agentBuilderEnabled / orgMemoriesEnabled: these tools are never
// granted implicitly.
func skillManagerEnabled(agent *model.Agent) bool {
	if agent == nil {
		return false
	}
	if agent.IsDefault {
		return true
	}
	if agent.McpToolFilter == nil {
		return false
	}
	for _, allowed := range agent.McpToolFilter.Allow {
		switch strings.TrimSpace(allowed) {
		case toolCreateOrgPlugin, toolCreateSkill, toolUpdateSkill, toolArchiveSkill:
			return true
		}
	}
	return false
}

func registerSkillManagerTools(server *mcp.Server, db *gorm.DB, token *model.Token, frontendURL string) {
	registerCreateOrgPluginTool(server, db, token)
	registerCreateSkillTool(server, db, token, frontendURL)
	registerUpdateSkillTool(server, db, token, frontendURL)
	registerArchiveSkillTool(server, db, token)
}

// --- create_org_plugin ---------------------------------------------------------

type createOrgPluginArgs struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Icon        string `json:"icon"`
	IconColor   string `json:"icon_color"`
}

func registerCreateOrgPluginTool(server *mcp.Server, db *gorm.DB, token *model.Token) {
	server.AddTool(&mcp.Tool{
		Name:        toolCreateOrgPlugin,
		Description: "Create an org-owned plugin: a named group (e.g. \"Sales\", \"Engineering\") that holds this organization's custom skills. Skills always live inside a plugin — create or reuse one before calling create_skill. The plugin is installed on the org immediately; attach it to agents with update_agent(plugin_slugs). Check list_org_plugins first to reuse an existing group instead of creating a near-duplicate.",
		InputSchema: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"name":        map[string]any{"type": "string", "description": "Display name for the plugin group, e.g. \"Sales\" or \"Engineering\". The slug is generated from it."},
				"description": map[string]any{"type": "string", "description": "Short description of what the skills in this group are for."},
				"category":    map[string]any{"type": "string", "description": "Optional catalog category. Defaults to \"Custom\"."},
				"icon":        map[string]any{"type": "string", "description": "Optional lucide icon name, e.g. \"briefcase\" or \"wrench\"."},
				"icon_color":  map[string]any{"type": "string", "description": "Optional hex icon color, e.g. \"#0EA5E9\"."},
			},
			"required": []string{"name"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args createOrgPluginArgs
		if err := decodeSkillToolArgs(req, &args); err != nil {
			return skillToolError(err.Error()), nil
		}
		return handleCreateOrgPlugin(ctx, db, token, args)
	})
}

func handleCreateOrgPlugin(ctx context.Context, db *gorm.DB, token *model.Token, args createOrgPluginArgs) (*mcp.CallToolResult, error) {
	name := strings.TrimSpace(args.Name)
	if name == "" {
		return skillToolError("name is required"), nil
	}
	if len(name) > maxSkillNameLen {
		return skillToolError(fmt.Sprintf("name must be at most %d characters", maxSkillNameLen)), nil
	}
	slug := model.GenerateSlug(name)
	category := strings.TrimSpace(args.Category)
	if category == "" {
		category = "Custom"
	}

	// Reject slugs that collide with any plugin visible to this org — a global
	// catalog plugin or one of the org's own (any status: the per-org unique
	// index covers archived rows too).
	var count int64
	if err := db.WithContext(ctx).Model(&model.Plugin{}).
		Where("slug = ? AND (org_id IS NULL OR org_id = ?)", slug, token.OrgID).
		Count(&count).Error; err != nil {
		return skillToolError("failed to check plugin slug: " + err.Error()), nil
	}
	if count > 0 {
		return skillToolError(fmt.Sprintf("a plugin with slug %q already exists for this org; reuse it (see list_org_plugins) or pick a different name", slug)), nil
	}

	manifest := map[string]any{
		"version":     1,
		"slug":        slug,
		"name":        name,
		"description": strings.TrimSpace(args.Description),
		"category":    category,
		"icon":        strings.TrimSpace(args.Icon),
		"icon_color":  strings.TrimSpace(args.IconColor),
		"org_plugin":  true,
	}
	rawManifest, err := json.Marshal(manifest)
	if err != nil {
		return skillToolError("failed to build plugin manifest"), nil
	}

	orgID := token.OrgID
	plugin := model.Plugin{
		ID:          uuid.New(),
		OrgID:       &orgID,
		Slug:        slug,
		Name:        name,
		Description: strings.TrimSpace(args.Description),
		Category:    category,
		Icon:        strings.TrimSpace(args.Icon),
		IconColor:   strings.TrimSpace(args.IconColor),
		Developer:   "Custom",
		Status:      model.PluginStatusActive,
		Manifest:    model.RawJSON(rawManifest),
	}
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&plugin).Error; err != nil {
			return fmt.Errorf("create plugin: %w", err)
		}
		install := model.OrgPluginInstall{ID: uuid.New(), OrgID: orgID, PluginID: plugin.ID}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&install).Error; err != nil {
			return fmt.Errorf("install plugin for org: %w", err)
		}
		return nil
	})
	if err != nil {
		return skillToolError(err.Error()), nil
	}

	return skillToolJSON(map[string]any{
		"success": true,
		"plugin": map[string]any{
			"id":       plugin.ID.String(),
			"slug":     plugin.Slug,
			"name":     plugin.Name,
			"category": plugin.Category,
		},
		"hint": "Plugin created and installed on the org. Add skills with create_skill(plugin_slug=\"" + plugin.Slug + "\"). To make agents use them, attach the plugin with update_agent — plugin_slugs REPLACES the agent's set, so call get_agent first and include its existing plugins.",
	})
}
