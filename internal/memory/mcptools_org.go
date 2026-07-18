package memory

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/usehivy/hivy/internal/model"
)

const manageMemoriesToolName = "manage_memories"

const manageMemoriesTopTagsLimit = 25

const manageMemoriesDescription = `Read-only memory view for this agent.

action search runs a semantic search over this agent's consolidated observations. Set include_facts true to inspect raw extracted facts.

action overview returns aggregate counts and top tags for this agent.

Memory is read-only to agents: new memories are written automatically by background reflection over sessions, and corrections or deletions happen in the memories UI.`

type manageMemoriesArgs struct {
	Action          string   `json:"action"`
	Query           string   `json:"query"`
	Tags            []string `json:"tags"`
	IncludeFacts    bool     `json:"include_facts"`
	Limit           int      `json:"limit"`
	HivySessionID   string   `json:"_hivy_session_id"`
	HivyActorUserID string   `json:"_hivy_actor_user_id"`
}

func registerManageMemoriesTool(server *mcp.Server, service *Service, token *model.Token, agentID uuid.UUID) {
	server.AddTool(&mcp.Tool{
		Name:        manageMemoriesToolName,
		Description: manageMemoriesDescription,
		InputSchema: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"action": map[string]any{
					"type":        "string",
					"enum":        []string{"search", "overview"},
					"description": "search runs a semantic search over this agent's memory; overview returns aggregate counts.",
				},
				"query": map[string]any{
					"type":        "string",
					"description": "Required for action search. Short semantic search phrase, max 6 words and 40 characters.",
					"maxLength":   memoryToolQueryMaxChars,
				},
				"tags":          memoryTagsSchema("Optional lowercase kebab-case slug filters for search."),
				"include_facts": memoryIncludeFactsSchema(),
				"limit": map[string]any{
					"type":        "integer",
					"minimum":     1,
					"maximum":     50,
					"description": "Optional for action search. Maximum results to return, default 10, max 50.",
				},
			},
			"required": []string{"action"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args manageMemoriesArgs
		if err := decodeMemoryToolArgs(req, &args); err != nil {
			return memoryToolError(err.Error()), nil
		}
		switch strings.ToLower(strings.TrimSpace(args.Action)) {
		case "search":
			return handleManageSearch(ctx, service, token, agentID, args)
		case "overview", "list":
			return handleManageOverview(ctx, service, token, agentID)
		default:
			return memoryToolError("action must be search or overview"), nil
		}
	})
}
