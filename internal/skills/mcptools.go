package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

const skillsListDescription = "List available skills (name + description). Call skill_view(name) to load a skill's full instructions; loading also materializes its files (references, scripts) into .skills/<name> in your workspace."

const skillViewDescription = "Load a skill's full content and materialize its bundle (SKILL.md plus references/, templates/, scripts/, assets/) into .skills/<name> in your workspace so linked files and scripts are usable. Pass file_path to read a single linked file instead."

// NewToolsFunc registers the read-only skill tools (skills_list, skill_view)
// for agent proxy MCP servers. Skills are DB-backed and scoped to the token's
// agent/org; skill_view returns a materialize payload the runtime writes to
// the sandbox workspace.
func NewToolsFunc(db *gorm.DB) func(server *mcp.Server, token *model.Token) {
	return func(server *mcp.Server, token *model.Token) {
		if server == nil || db == nil || !skillToolAgentProxy(token) {
			return
		}
		agentID, err := skillToolAgentID(token)
		if err != nil {
			return
		}
		registerSkillsListTool(server, db, token, agentID)
		registerSkillViewTool(server, db, token, agentID)
	}
}

type skillsListArgs struct {
	Category string `json:"category"`
}

type skillViewArgs struct {
	Name     string `json:"name"`
	FilePath string `json:"file_path"`
}

func registerSkillsListTool(server *mcp.Server, db *gorm.DB, token *model.Token, agentID uuid.UUID) {
	server.AddTool(&mcp.Tool{
		Name:        "skills_list",
		Description: skillsListDescription,
		InputSchema: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"category": map[string]any{
					"type":        "string",
					"description": "Optional category filter to narrow results.",
				},
			},
			"required": []string{},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args skillsListArgs
		if err := decodeSkillToolArgs(req, &args); err != nil {
			return skillToolError(err.Error()), nil
		}
		return handleSkillsList(ctx, db, token, agentID, args)
	})
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
					"description": "The skill name (use skills_list to see available skills).",
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

func handleSkillsList(ctx context.Context, db *gorm.DB, token *model.Token, agentID uuid.UUID, args skillsListArgs) (*mcp.CallToolResult, error) {
	agent, err := loadActiveAgent(ctx, db, token.OrgID, agentID)
	if err != nil {
		return skillToolError(err.Error()), nil
	}
	summaries, err := AgentSkillSummaries(ctx, db, agent)
	if err != nil {
		return skillToolError("failed to load skills: " + err.Error()), nil
	}
	category := strings.TrimSpace(args.Category)

	items := make([]map[string]any, 0, len(summaries))
	categories := map[string]bool{}
	for _, summary := range summaries {
		if summary.Category != "" {
			categories[summary.Category] = true
		}
		if category != "" && summary.Category != category {
			continue
		}
		items = append(items, map[string]any{
			"name":        summary.Name,
			"description": summary.Description,
			"category":    nullableString(summary.Category),
		})
	}

	return skillToolJSON(map[string]any{
		"success":    true,
		"skills":     items,
		"categories": sortedKeys(categories),
		"count":      len(items),
		"hint":       "Call skill_view(name) to load a skill and materialize its files into .skills/<name>.",
	})
}

func handleSkillView(ctx context.Context, db *gorm.DB, token *model.Token, agentID uuid.UUID, args skillViewArgs) (*mcp.CallToolResult, error) {
	name := strings.TrimSpace(args.Name)
	if name == "" {
		return skillToolError("name is required"), nil
	}
	agent, err := loadActiveAgent(ctx, db, token.OrgID, agentID)
	if err != nil {
		return skillToolError(err.Error()), nil
	}
	all, err := loadAgentPublishedSkills(ctx, db, agentID)
	if err != nil {
		return skillToolError("failed to load skills: " + err.Error()), nil
	}
	filter := resolveSkillFilter(ctx, db, agent)

	var found *model.Skill
	for i := range all {
		if all[i].Slug == name && skillAllowed(all[i].Slug, filter) {
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

func sortedKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for key := range set {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}
