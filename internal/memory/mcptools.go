package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/usehivy/hivy/internal/model"
)

const (
	memoryToolOwnerOrg  = "org"
	memoryToolOwnerUser = "user"
	memoryToolOwnerBoth = "both"

	memoryToolSearchLimit   = 10
	memoryToolQueryMaxWords = 6
	memoryToolQueryMaxChars = 40
	memoryToolMaxTags       = 5
)

// NewToolsFunc registers memory tools for agent proxy MCP servers.
//
// search_memories, retain_memory, and forget_memory always register for agent
// proxy tokens. The privileged org_memories supervisor tool (and forget_memory's
// user_approved cross-agent path) additionally require the calling agent to be
// the org's default agent or explicitly allow-listed (see orgMemoriesEnabled);
// the org-scoped agent row is loaded once here to evaluate that gate. A failed
// agent load never blocks the three base tools.
func NewToolsFunc(service *Service) func(server *mcp.Server, token *model.Token) {
	return func(server *mcp.Server, token *model.Token) {
		if server == nil || service == nil || service.cfg.DB == nil || !memoryToolAgentProxy(token) {
			return
		}
		agentID, err := memoryToolAgentID(token)
		if err != nil {
			return
		}
		supervisor := false
		if agent, err := service.loadOrgAgent(context.Background(), token.OrgID, agentID); err == nil {
			supervisor = orgMemoriesEnabled(agent)
		}
		registerSearchMemoriesTool(server, service, token, agentID)
		registerRetainMemoryTool(server, service, token, agentID)
		registerForgetMemoryTool(server, service, token, agentID, supervisor)
		if supervisor {
			registerOrgMemoriesTool(server, service, token)
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
					"description": "Short semantic search phrase, max 6 words and 40 characters. Example: helio launch checklist.",
					"maxLength":   memoryToolQueryMaxChars,
				},
				"target": memorySearchTargetSchema(),
				"tags":   memoryTagsSchema("Optional exact filters using lowercase kebab-case slugs such as project-helio or billing."),
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

func registerRetainMemoryTool(server *mcp.Server, service *Service, token *model.Token, agentID uuid.UUID) {
	server.AddTool(&mcp.Tool{
		Name:        "retain_memory",
		Description: retainMemoryDescription,
		InputSchema: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"content": map[string]any{
					"type":        "string",
					"description": "Concise durable memory to store. Write one stable fact, preference, rule, or decision per call.",
				},
				"target":   memoryRetainTargetSchema(),
				"tags":     memoryTagsSchema("Optional lowercase kebab-case labels for filtering later, max 5. Examples: preference, project-helio."),
				"metadata": map[string]any{"type": "object", "description": "Optional small JSON object for non-sensitive source metadata."},
			},
			"required": []string{"content", "target"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args memoryRetainArgs
		if err := decodeMemoryToolArgs(req, &args); err != nil {
			return memoryToolError(err.Error()), nil
		}
		return handleRetainMemory(ctx, service, token, agentID, args)
	})
}

func registerForgetMemoryTool(server *mcp.Server, service *Service, token *model.Token, agentID uuid.UUID, supervisor bool) {
	server.AddTool(&mcp.Tool{
		Name:        "forget_memory",
		Description: forgetMemoryDescription,
		InputSchema: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"memory_id": map[string]any{
					"type":        "string",
					"description": "UUID returned by search_memories or retain_memory.",
				},
				"reason": map[string]any{
					"type":        "string",
					"description": "Brief reason the memory is wrong, stale, duplicated, or unsafe to keep.",
				},
				"user_approved": map[string]any{
					"type":        "boolean",
					"description": "Only honored for the org's default agent. Required when archiving a memory that belongs to another agent. Set it ONLY after showing the user the memory's content and owning agent and receiving explicit confirmation in this session.",
				},
			},
			"required": []string{"memory_id"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args memoryForgetArgs
		if err := decodeMemoryToolArgs(req, &args); err != nil {
			return memoryToolError(err.Error()), nil
		}
		return handleForgetMemory(ctx, service, token, agentID, supervisor, args)
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
	target, err := normalizeMemorySearchTarget(args.Target, toolCtx)
	if err != nil {
		return memoryToolError(err.Error()), nil
	}
	tags, err := normalizeMemoryToolTags(args.Tags)
	if err != nil {
		return memoryToolError(err.Error()), nil
	}
	searchCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	hits, err := service.Search(searchCtx, SearchRequest{
		OrgID:           token.OrgID,
		UserID:          target.UserID,
		AgentID:         agentID,
		Scope:           target.Scope,
		AgentVisibility: target.Visibility,
		Query:           query,
		Tags:            tags,
		Limit:           memoryToolSearchLimit,
	})
	if err != nil {
		return memoryToolError("memory search failed: " + err.Error()), nil
	}
	return memoryToolJSON(map[string]any{
		"success": true,
		"query":   query,
		"target":  target.Response,
		"results": memoryToolSearchResponses(hits),
		"total":   len(hits),
	})
}

func handleRetainMemory(ctx context.Context, service *Service, token *model.Token, agentID uuid.UUID, args memoryRetainArgs) (*mcp.CallToolResult, error) {
	toolCtx, err := service.memoryToolContext(ctx, token, agentID, args.HivySessionID)
	if err != nil {
		return memoryToolError(err.Error()), nil
	}
	target, err := service.normalizeMemoryRetainTarget(ctx, args.Target, toolCtx)
	if err != nil {
		return memoryToolError(err.Error()), nil
	}
	tags, err := normalizeMemoryToolTags(args.Tags)
	if err != nil {
		return memoryToolError(err.Error()), nil
	}
	createCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	mem, err := service.Create(createCtx, CreateRequest{
		OrgID:           token.OrgID,
		AgentID:         target.AgentID,
		UserID:          target.UserID,
		Scope:           target.Scope,
		Content:         args.Content,
		Tags:            tags,
		Metadata:        memoryToolMetadata(args.Metadata, target.Response, agentID),
		SourceSessionID: toolCtx.SessionID,
		CreatedByUserID: toolCtx.UserID,
	})
	if err != nil {
		return memoryToolError("retain_memory failed: " + err.Error()), nil
	}
	return memoryToolJSON(map[string]any{
		"success": true,
		"memory":  memoryToolMemoryResponse(*mem, nil),
		"note":    "Embedding is queued asynchronously; semantic search becomes available after embedding_status is ready.",
	})
}

func handleForgetMemory(ctx context.Context, service *Service, token *model.Token, agentID uuid.UUID, supervisor bool, args memoryForgetArgs) (*mcp.CallToolResult, error) {
	memoryID, err := uuid.Parse(strings.TrimSpace(args.MemoryID))
	if err != nil || memoryID == uuid.Nil {
		return memoryToolError("memory_id must be a valid UUID"), nil
	}
	if args.UserApproved && !supervisor {
		return memoryToolError("user_approved is only honored for the org's default agent"), nil
	}
	toolCtx, err := service.memoryToolContext(ctx, token, agentID, args.HivySessionID)
	if err != nil {
		return memoryToolError(err.Error()), nil
	}
	mem, err := service.loadToolMemory(ctx, token.OrgID, memoryID)
	if err != nil {
		return memoryToolError(memoryToolLoadMessage(err)), nil
	}
	if err := toolCtx.canForget(mem, supervisor, args.UserApproved); err != nil {
		return memoryToolError(err.Error()), nil
	}
	archiveReq := ArchiveRequest{OrgID: token.OrgID, ID: memoryID, AgentID: &agentID}
	if mem.AgentID != nil && *mem.AgentID != agentID {
		// Only reachable after canForget approved the supervisor+user_approved
		// cross-agent path; lift the agent-scoped SQL guard so the archive can
		// reach the other agent's memory.
		archiveReq.AgentID = nil
	}
	if err := service.Archive(ctx, archiveReq); err != nil {
		return memoryToolError("forget_memory failed: " + err.Error()), nil
	}
	return memoryToolJSON(map[string]any{
		"success":   true,
		"memory_id": memoryID.String(),
		"status":    "archived",
	})
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
