package skills

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

// --- create_skill --------------------------------------------------------------

type createSkillArgs struct {
	EntrypointContent string            `json:"entrypoint_content"`
	Files             map[string]string `json:"files"`
	HivyActorUserID   string            `json:"_hivy_actor_user_id"`
}

func registerCreateSkillTool(server *mcp.Server, db *gorm.DB, token *model.Token, frontendURL string) {
	server.AddTool(&mcp.Tool{
		Name:        toolCreateSkill,
		Description: "Create and immediately publish a skill bundle owned by the calling agent's team. The runtime asks for an explicit SKILL.md entrypoint path and optional supporting file paths under references/, templates/, scripts/, or assets/. SKILL.md is the instruction entrypoint agents load first. Its YAML frontmatter requires a lowercase kebab-case name and agent-facing description, and may include human_description, category, tags, and required_environment_variables. Get the user's explicit approval of the final bundle before calling this.",
		InputSchema: createSkillSchema(),
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args createSkillArgs
		if err := decodeSkillToolArgs(req, &args); err != nil {
			return skillToolError(err.Error()), nil
		}
		return handleCreateSkill(ctx, db, token, frontendURL, args)
	})
}

func createSkillSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"entrypoint_content": map[string]any{"type": "string", "description": "Complete UTF-8 SKILL.md entrypoint including YAML frontmatter. Supplied from entrypoint_file_path by the sandbox runtime."},
			"files": map[string]any{
				"type":                 "object",
				"additionalProperties": map[string]any{"type": "string"},
				"description":          "UTF-8 supporting files keyed by bundle-relative path. Supplied from supporting_file_paths by the sandbox runtime.",
			},
		},
		"required": []string{"entrypoint_content"},
	}
}

func handleCreateSkill(ctx context.Context, db *gorm.DB, token *model.Token, frontendURL string, args createSkillArgs) (*mcp.CallToolResult, error) {
	agent, actor, errResult := resolveSkillManagerTeam(ctx, db, token, args.HivyActorUserID, "creating a skill")
	if errResult != nil {
		return errResult, nil
	}
	parsed, errResult := parseSkillToolBundle(args.EntrypointContent, args.Files)
	if errResult != nil {
		return errResult, nil
	}
	slug := parsed.Name

	var count int64
	if err := db.WithContext(ctx).Model(&model.Skill{}).
		Where("org_id = ? AND team_id = ? AND slug = ?", token.OrgID, agent.TeamID, slug).
		Count(&count).Error; err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "check team skill slug", "error", err)
		return skillToolError("failed to check skill slug"), nil
	}
	if count > 0 {
		return skillToolError(fmt.Sprintf("skill %q already exists on this team; use update_skill to change it", slug)), nil
	}

	rawBundle, err := marshalSkillBundle(
		slug,
		parsed.Name,
		parsed.Description,
		parsed.Content,
		parsed.Files,
		parsed.RequiredEnvironmentVariables,
	)
	if err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "serialize team skill bundle", "error", err)
		return skillToolError("failed to serialize skill bundle"), nil
	}

	now := time.Now()
	orgID := token.OrgID
	skill := model.Skill{
		ID:               uuid.New(),
		OrgID:            &orgID,
		TeamID:           &agent.TeamID,
		PublisherID:      &actor.UserID,
		Slug:             slug,
		Name:             parsed.Name,
		Description:      &parsed.Description,
		HumanDescription: parsed.HumanDescription,
		Category:         parsed.Category,
		SourceType:       model.SkillSourceInline,
		RepoRef:          "main",
		Bundle:           model.RawJSON(rawBundle),
		Tags:             pq.StringArray(parsed.Tags),
		Status:           model.SkillStatusPublished,
		HydratedAt:       &now,
	}
	if err := db.WithContext(ctx).Create(&skill).Error; err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "create team skill", "error", err)
		return skillToolError("failed to create skill"), nil
	}

	return skillToolJSON(map[string]any{
		"success": true,
		"skill": map[string]any{
			"slug":    skill.Slug,
			"name":    skill.Name,
			"team_id": agent.TeamID.String(),
			"status":  skill.Status,
		},
		"required_environment_variables": parsed.RequiredEnvironmentVariables,
		"environment_settings_url":       environmentSettingsURL(frontendURL),
		"hint":                           skillPublishHint(),
	})
}

// --- update_skill --------------------------------------------------------------

type updateSkillArgs struct {
	Skill             string            `json:"skill"`
	EntrypointContent string            `json:"entrypoint_content"`
	Files             map[string]string `json:"files"`
	HivyActorUserID   string            `json:"_hivy_actor_user_id"`
}

func registerUpdateSkillTool(server *mcp.Server, db *gorm.DB, token *model.Token, frontendURL string) {
	server.AddTool(&mcp.Tool{
		Name:        toolUpdateSkill,
		Description: "Replace and immediately publish the complete bundle for a skill owned by the calling agent's team. The runtime asks for an explicit SKILL.md entrypoint path and optional supporting file paths; omitted supporting files are removed. SKILL.md YAML frontmatter carries description, human_description, category, tags, and required_environment_variables, and its name must match the immutable skill slug. Updating an archived skill republishes it. Get the user's explicit approval of the final bundle before calling this.",
		InputSchema: updateSkillSchema(),
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args updateSkillArgs
		if err := decodeSkillToolArgs(req, &args); err != nil {
			return skillToolError(err.Error()), nil
		}
		return handleUpdateSkill(ctx, db, token, frontendURL, args)
	})
}

func updateSkillSchema() map[string]any {
	schema := createSkillSchema()
	props := schema["properties"].(map[string]any)
	props["skill"] = map[string]any{"type": "string", "description": "Slug of the team skill to update."}
	schema["required"] = []string{"skill", "entrypoint_content"}
	return schema
}

func handleUpdateSkill(ctx context.Context, db *gorm.DB, token *model.Token, frontendURL string, args updateSkillArgs) (*mcp.CallToolResult, error) {
	agent, _, errResult := resolveSkillManagerTeam(ctx, db, token, args.HivyActorUserID, "updating a skill")
	if errResult != nil {
		return errResult, nil
	}
	skill, errResult := loadTeamOwnedSkill(ctx, db, token.OrgID, agent.TeamID, args.Skill)
	if errResult != nil {
		return errResult, nil
	}
	parsed, errResult := parseSkillToolBundle(args.EntrypointContent, args.Files)
	if errResult != nil {
		return errResult, nil
	}
	if parsed.Name != skill.Slug {
		return skillToolError(fmt.Sprintf("SKILL.md frontmatter name must remain %q when updating this skill", skill.Slug)), nil
	}

	rawBundle, err := marshalSkillBundle(
		skill.Slug,
		parsed.Name,
		parsed.Description,
		parsed.Content,
		parsed.Files,
		parsed.RequiredEnvironmentVariables,
	)
	if err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "serialize team skill bundle", "error", err)
		return skillToolError("failed to serialize skill bundle"), nil
	}

	now := time.Now()
	updates := map[string]any{
		"name":              parsed.Name,
		"description":       &parsed.Description,
		"human_description": parsed.HumanDescription,
		"category":          parsed.Category,
		"tags":              pq.StringArray(parsed.Tags),
		"bundle":            model.RawJSON(rawBundle),
		"status":            model.SkillStatusPublished,
		"hydrated_at":       &now,
	}
	if err := db.WithContext(ctx).Model(skill).Updates(updates).Error; err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "update team skill", "error", err)
		return skillToolError("failed to update skill"), nil
	}

	return skillToolJSON(map[string]any{
		"success": true,
		"skill": map[string]any{
			"slug":    skill.Slug,
			"name":    parsed.Name,
			"team_id": agent.TeamID.String(),
			"status":  model.SkillStatusPublished,
		},
		"required_environment_variables": parsed.RequiredEnvironmentVariables,
		"environment_settings_url":       environmentSettingsURL(frontendURL),
		"hint":                           skillPublishHint(),
	})
}
