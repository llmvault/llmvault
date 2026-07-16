package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

const skillViewDescription = "Load a skill's full content and materialize its bundle (SKILL.md plus references/, templates/, scripts/, assets/) into .skills/<name> in your workspace so linked files and scripts are usable. Pass file_path to read a single linked file instead."

// NewToolsFunc registers skill_view for agent proxy MCP servers. The static
// system prompt already lists every available skill, so a separate skills_list
// tool would only repeat that inventory in each MCP tool catalog. Skills are
// DB-backed and scoped to the token's agent/org; skill_view returns a
// materialize payload the runtime writes to the sandbox workspace.
//
// It also registers mutating skill tools when the calling agent's managed MCP
// filter allows them. frontendURL builds the environment-settings link.
func NewToolsFunc(db *gorm.DB, frontendURL string) func(server *mcp.Server, token *model.Token) {
	return func(server *mcp.Server, token *model.Token) {
		if server == nil || db == nil || !skillToolAgentProxy(token) {
			return
		}
		agentID, err := skillToolAgentID(token)
		if err != nil {
			return
		}
		registerSkillViewTool(server, db, token, agentID)

		agent, err := loadActiveAgent(context.Background(), db, token.OrgID, agentID)
		if err != nil || !skillManagerEnabled(agent) {
			return
		}
		registerSkillManagerTools(server, db, token, frontendURL)
	}
}

type skillViewArgs struct {
	Name     string `json:"name"`
	FilePath string `json:"file_path"`
}

func registerSkillViewTool(server *mcp.Server, db *gorm.DB, token *model.Token, agentID uuid.UUID) {
	server.AddTool(&mcp.Tool{
		Name:        "skill_view",
		Description: skillViewDescription,
		InputSchema: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "The skill name from your Available skills system-prompt section.",
				},
				"file_path": map[string]any{
					"type":        "string",
					"description": "Optional linked file path within the skill, e.g. references/api.md or scripts/check.sh.",
				},
			},
			"required": []string{"name"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args skillViewArgs
		if err := decodeSkillToolArgs(req, &args); err != nil {
			return skillToolError(err.Error()), nil
		}
		return handleSkillView(ctx, db, token, agentID, args)
	})
}

func handleSkillView(ctx context.Context, db *gorm.DB, token *model.Token, agentID uuid.UUID, args skillViewArgs) (*mcp.CallToolResult, error) {
	name := strings.TrimSpace(args.Name)
	if name == "" {
		return skillToolError("name is required"), nil
	}
	_, err := loadActiveAgent(ctx, db, token.OrgID, agentID)
	if err != nil {
		return skillToolError(err.Error()), nil
	}
	all, err := loadAgentPublishedSkills(ctx, db, agentID)
	if err != nil {
		return skillToolError("failed to load skills: " + err.Error()), nil
	}
	var found *model.Skill
	for i := range all {
		if all[i].Slug == name {
			found = &all[i]
			break
		}
	}
	if found == nil {
		return skillToolError(fmt.Sprintf("skill %q not found", name)), nil
	}
	bundle, err := decodeSkillBundle(*found)
	if err != nil {
		return skillToolError(err.Error()), nil
	}

	// Read a single linked file without materializing the whole bundle.
	if filePath := strings.TrimSpace(args.FilePath); filePath != "" {
		files := skillBundleFiles(bundle)
		content, ok := files[filePath]
		if !ok {
			return skillToolError(fmt.Sprintf("file %q not found in skill %q", filePath, name)), nil
		}
		return skillToolJSON(map[string]any{
			"success": true,
			"name":    found.Slug,
			"file":    filePath,
			"content": content,
		})
	}

	// Full view: model-facing summary in text content, materialize payload in
	// structured content for the runtime to write to disk.
	requiredEnv := bundle.RequiredEnvironmentVariables
	if requiredEnv == nil {
		requiredEnv = []string{}
	}
	summary := map[string]any{
		"success":                        true,
		"name":                           found.Slug,
		"description":                    skillDescription(*found, bundle),
		"category":                       nullableString(found.Category),
		"tags":                           []string(found.Tags),
		"content":                        composeSkillMarkdown(*found, bundle),
		"skill_dir":                      ".skills/" + found.Slug,
		"linked_files":                   linkedFileGroups(bundle),
		"required_environment_variables": requiredEnv,
		"usage_hint":                     "Files are materialized under .skills/" + found.Slug + " in your workspace. Read linked files there, or call skill_view(name, file_path) to fetch one directly.",
	}
	body, err := json.Marshal(summary)
	if err != nil {
		return skillToolError("failed to serialize skill"), nil
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(body)}},
		StructuredContent: map[string]any{
			"materialize": materializePayload(*found, bundle),
		},
	}, nil
}

func decodeSkillToolArgs(req *mcp.CallToolRequest, dst any) error {
	if req == nil || req.Params.Arguments == nil {
		return fmt.Errorf("arguments are required")
	}
	if err := json.Unmarshal(req.Params.Arguments, dst); err != nil {
		return fmt.Errorf("invalid arguments")
	}
	return nil
}

func skillToolAgentProxy(token *model.Token) bool {
	if token == nil || token.Meta == nil {
		return false
	}
	tokenType, _ := token.Meta[model.TokenMetaType].(string)
	return tokenType == model.TokenTypeAgentProxy
}

func skillToolAgentID(token *model.Token) (uuid.UUID, error) {
	agentIDText, _ := token.Meta[model.TokenMetaAgentID].(string)
	agentID, err := uuid.Parse(strings.TrimSpace(agentIDText))
	if err != nil || agentID == uuid.Nil {
		return uuid.Nil, fmt.Errorf("agent proxy token is missing agent_id")
	}
	return agentID, nil
}

func skillToolError(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: "Error: " + msg}},
		IsError: true,
	}
}

func skillToolJSON(v any) (*mcp.CallToolResult, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return skillToolError("failed to serialize response"), nil
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(b)}}}, nil
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
