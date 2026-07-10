package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/usehivy/hivy/internal/model"
)

const (
	memoryToolSearchLimit   = 10
	memoryToolQueryMaxWords = 6
	memoryToolQueryMaxChars = 40
	memoryToolMaxTags       = 5
)

// NewToolsFunc registers memory tools for agent proxy MCP servers.
//
// Agents are read-only on memory: every write flows through background
// reflection/consolidation, and corrections or deletions happen in the
// memories UI. search_memories is temporarily not mounted (see
// searchMemoriesToolMounted); when enabled it registers for every agent proxy
// token and is auto-scoped to the session's channel. The privileged manage_memories tool
// (also read-only: search + overview) additionally requires the calling agent
// to be the org's default agent; the agent row is loaded once here to
// evaluate that gate. A failed agent load never blocks the base tool.
//
// searchMemoriesToolMounted is intentionally false: an agent's memories are
// already auto-injected into its context, so the search_memories MCP tool is
// redundant, and agents were observed confusing it with knowledge-base/session
// search (and calling it against the currently-unreliable embed path).
// Temporarily unmounted — registerSearchMemoriesTool and handleSearchMemories
// below are kept intact; flip this to true to re-enable.
const searchMemoriesToolMounted = false

func NewToolsFunc(service *Service) func(server *mcp.Server, token *model.Token) {
	return func(server *mcp.Server, token *model.Token) {
		if server == nil || service == nil || service.cfg.DB == nil || !memoryToolAgentProxy(token) {
			return
		}
		agentID, err := memoryToolAgentID(token)
		if err != nil {
			return
		}
		manage := false
		if agent, err := service.loadOrgAgent(context.Background(), token.OrgID, agentID); err == nil {
			manage = agent.IsDefault
		}
		if searchMemoriesToolMounted {
			registerSearchMemoriesTool(server, service, token, agentID)
		}
		if manage {
			registerManageMemoriesTool(server, service, token)
		}
	}
}

func registerSearchMemoriesTool(server *mcp.Server, service *Service, token *model.Token, agentID uuid.UUID) {
	server.AddTool(&mcp.Tool{
		Name:        "search_memories",
		Description: searchMemoriesDescription,
		InputSchema: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "2-6 word noun phrase (max 40 characters) worded like the remembered fact itself, naming concrete entities. Examples: helio launch checklist, refund approval policy. One concept per call; never a full question or sentence.",
					"maxLength":   memoryToolQueryMaxChars,
				},
				"tags":          memoryTagsSchema("Optional exact filters using lowercase kebab-case slugs such as project-helio or billing."),
				"include_facts": memoryIncludeFactsSchema(),
			},
			"required": []string{"query"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args memorySearchArgs
		if err := decodeMemoryToolArgs(req, &args); err != nil {
			return memoryToolError(err.Error()), nil
		}
		return handleSearchMemories(ctx, service, token, agentID, args)
	})
}

func handleSearchMemories(ctx context.Context, service *Service, token *model.Token, agentID uuid.UUID, args memorySearchArgs) (*mcp.CallToolResult, error) {
	query, err := normalizeMemoryToolSearchQuery(args.Query)
	if err != nil {
		return memoryToolError(err.Error()), nil
	}
	toolCtx, err := service.memoryToolContext(ctx, token, agentID, args.HivySessionID)
	if err != nil {
		return memoryToolError(err.Error()), nil
	}
	tags, err := normalizeMemoryToolTags(args.Tags)
	if err != nil {
		return memoryToolError(err.Error()), nil
	}
	searchCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	searchReq := SearchRequest{
		OrgID: token.OrgID,
		Scope: toolCtx.searchScope(),
		Query: query,
		Tags:  tags,
		Limit: memoryToolSearchLimit,
	}
	if args.IncludeFacts {
		hits, err := service.Search(searchCtx, searchReq)
		if err != nil {
			return memoryToolError("memory search failed: " + err.Error()), nil
		}
		return memoryToolJSON(map[string]any{
			"success":    true,
			"query":      query,
			"channel_id": toolCtx.ChannelID.String(),
			"layer":      memoryLayerFacts,
			"results":    memoryToolSearchResponses(hits),
			"total":      len(hits),
		})
	}
	hits, err := service.SearchObservations(searchCtx, searchReq)
	if err != nil {
		return memoryToolError("memory search failed: " + err.Error()), nil
	}
	out := map[string]any{
		"success":    true,
		"query":      query,
		"channel_id": toolCtx.ChannelID.String(),
		"layer":      memoryLayerObservations,
		"results":    observationToolSearchResponses(hits),
		"total":      len(hits),
	}
	if len(hits) == 0 {
		out["note"] = "No stored memory cleared the relevance threshold. Treat this as nothing being stored about the topic; do not retry with looser or broader wording. If you used tags, retry once without them."
	}
	return memoryToolJSON(out)
}

func decodeMemoryToolArgs(req *mcp.CallToolRequest, dst any) error {
	if req == nil || req.Params.Arguments == nil {
		return fmt.Errorf("arguments are required")
	}
	if err := json.Unmarshal(req.Params.Arguments, dst); err != nil {
		return fmt.Errorf("invalid arguments")
	}
	return nil
}
