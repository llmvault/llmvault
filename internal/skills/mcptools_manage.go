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

	"github.com/usehivy/hivy/internal/access"
	"github.com/usehivy/hivy/internal/model"
)

// Tool names for the skill-manager group. All four are opt-in mutating tools
// gated per calling agent (see skillManagerEnabled).
const (
	toolCreateTeamPlugin = "create_team_plugin"
	toolCreateSkill      = "create_skill"
	toolUpdateSkill      = "update_skill"
	toolArchiveSkill     = "archive_skill"
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

// skillManagerEnabled reports whether the calling agent may use the mutating
// skill-manager tools once its team has enabled the Skill Manager plugin. The
// default Hivy agent is eligible for them; any other agent must explicitly
// allow-list one of them in McpToolFilter.Allow. Mirrors agentBuilderEnabled /
// orgMemoriesEnabled: these tools are never granted implicitly.
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
		case toolCreateTeamPlugin, toolCreateSkill, toolUpdateSkill, toolArchiveSkill:
			return true
		}
	}
	return false
}

// resolveSkillManagerTeam resolves the calling agent's team and authorizes the
// human actor against it. Every active team member may manage that team's custom
// plugins and skills; org managers retain their normal cross-team access. A run
// without a human actor fails closed because these calls publish executable
// instructions into every agent on the team.
func resolveSkillManagerTeam(ctx context.Context, db *gorm.DB, token *model.Token, rawActorUserID, action string) (*model.Agent, *access.Actor, *mcp.CallToolResult) {
	if token == nil {
		return nil, nil, skillToolError("missing agent token")
	}
	agentID, err := skillToolAgentID(token)
	if err != nil {
		return nil, nil, skillToolError(err.Error())
	}
	agent, err := loadActiveAgent(ctx, db, token.OrgID, agentID)
	if err != nil {
		return nil, nil, skillToolError("calling agent not found")
	}
	actor, err := access.Resolve(ctx, db, token.OrgID, rawActorUserID)
	if err != nil {
		return nil, nil, skillToolError(err.Error())
	}
	if actor == nil {
		return nil, nil, skillToolError("Not allowed: " + action + " must be done on behalf of a team member, but this run has no human actor.")
	}
	ok, err := actor.CanManageTeamResource(ctx, db, agent.TeamID)
	if err != nil {
		return nil, nil, skillToolError(err.Error())
	}
	if !ok {
		return nil, nil, skillToolError("Not allowed: " + action + " requires membership in the calling agent's team.")
	}
	return agent, actor, nil
}

func registerSkillManagerTools(server *mcp.Server, db *gorm.DB, token *model.Token, frontendURL string) {
	registerCreateTeamPluginTool(server, db, token)
	registerCreateSkillTool(server, db, token, frontendURL)
	registerUpdateSkillTool(server, db, token, frontendURL)
	registerArchiveSkillTool(server, db, token)
}

// --- create_team_plugin --------------------------------------------------------

type createTeamPluginArgs struct {
	Name            string `json:"name"`
	Description     string `json:"description"`
	Category        string `json:"category"`
	Icon            string `json:"icon"`
	IconColor       string `json:"icon_color"`
	HivyActorUserID string `json:"_hivy_actor_user_id"`
}

func registerCreateTeamPluginTool(server *mcp.Server, db *gorm.DB, token *model.Token) {
	server.AddTool(&mcp.Tool{
		Name:        toolCreateTeamPlugin,
		Description: "Create a custom plugin owned by the calling agent's team: a named group (e.g. \"Sales\", \"Engineering\") that holds that team's custom skills. It is installed and enabled for the team immediately, so every agent on the team inherits its published skills. Check list_team_plugins first and reuse an existing group instead of creating a near-duplicate.",
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
		var args createTeamPluginArgs
		if err := decodeSkillToolArgs(req, &args); err != nil {
			return skillToolError(err.Error()), nil
		}
		return handleCreateTeamPlugin(ctx, db, token, args)
	})
}

func handleCreateTeamPlugin(ctx context.Context, db *gorm.DB, token *model.Token, args createTeamPluginArgs) (*mcp.CallToolResult, error) {
	agent, actor, errResult := resolveSkillManagerTeam(ctx, db, token, args.HivyActorUserID, "creating a team plugin")
	if errResult != nil {
		return errResult, nil
	}
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

	// Reject collisions with a catalog plugin or another plugin on this team.
	// A different team may intentionally use the same group name.
	var count int64
	if err := db.WithContext(ctx).Model(&model.Plugin{}).
		Where("slug = ? AND (org_id IS NULL OR (org_id = ? AND team_id = ?))", slug, token.OrgID, agent.TeamID).
		Count(&count).Error; err != nil {
		return skillToolError("failed to check plugin slug: " + err.Error()), nil
	}
	if count > 0 {
		return skillToolError(fmt.Sprintf("a plugin with slug %q already exists in the catalog or on this team; reuse it (see list_team_plugins) or pick a different name", slug)), nil
	}

	manifest := map[string]any{
		"version":     1,
		"slug":        slug,
		"name":        name,
		"description": strings.TrimSpace(args.Description),
		"category":    category,
		"icon":        strings.TrimSpace(args.Icon),
		"icon_color":  strings.TrimSpace(args.IconColor),
		"team_plugin": true,
	}
	rawManifest, err := json.Marshal(manifest)
	if err != nil {
		return skillToolError("failed to build plugin manifest"), nil
	}

	orgID := token.OrgID
	plugin := model.Plugin{
		ID:          uuid.New(),
		OrgID:       &orgID,
		TeamID:      &agent.TeamID,
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
		install := model.OrgPluginInstall{ID: uuid.New(), OrgID: orgID, PluginID: plugin.ID, CreatedByUserID: &actor.UserID}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&install).Error; err != nil {
			return fmt.Errorf("install plugin for org: %w", err)
		}
		grant := model.TeamPlugin{ID: uuid.New(), OrgID: orgID, TeamID: agent.TeamID, PluginID: plugin.ID, EnabledBy: &actor.UserID}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&grant).Error; err != nil {
			return fmt.Errorf("enable plugin for team: %w", err)
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
			"team_id":  agent.TeamID.String(),
		},
		"hint": "Team plugin created and enabled. Add skills with create_skill(plugin_slug=\"" + plugin.Slug + "\"); published skills become available to this team's agents immediately.",
	})
}
