package memory

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/usehivy/hivy/internal/access"
	"github.com/usehivy/hivy/internal/model"
)

const orgMemoriesToolName = "org_memories"

// requireOrgManagerActor gates the org-wide memory supervisor on the acting
// human being an org owner/admin. A nil result means allowed. When there is no
// human actor (automated run, or a deploy before the runtime injects identity)
// the check is skipped so existing flows keep working. rawActorUserID is the
// runtime-injected `_hivy_actor_user_id`.
func (s *Service) requireOrgManagerActor(ctx context.Context, orgID uuid.UUID, rawActorUserID string) *mcp.CallToolResult {
	actor, err := access.Resolve(ctx, s.cfg.DB, orgID, rawActorUserID)
	if err != nil {
		return memoryToolError(err.Error())
	}
	if actor == nil {
		return nil
	}
	if !actor.IsOrgManager() {
		return memoryToolError("Not allowed: viewing the organization's memories across all agents requires an admin or owner. " +
			"The person you're acting for has the role \"" + actor.OrgRole + "\", which can only see this agent's own memories. " +
			"Ask an organization admin or owner if you need the org-wide view.")
	}
	return nil
}

const orgMemoriesTopTagsLimit = 25

const orgMemoriesDescription = `Org-wide memory supervisor view across ALL agents' org memories. Available only to the org's default agent (or agents explicitly allow-listed for it). User-scoped personal memories are always excluded from results.

action search performs a semantic search over every org-scoped memory in the organization: shared memories and every agent's private memories, with the owning agent identified per result. Use it before seeding a memory for another agent with retain_memory target.agent_id, to check whether that agent already carries it. query is a 2-6 word phrase. agent_id optionally narrows to one agent's memories. tags are optional exact lowercase slug filters.

action overview returns aggregate counts only: total org memories, shared count, per-agent counts, top tags, and a user-scoped total (count only, never content).`

type orgMemoriesArgs struct {
	Action          string   `json:"action"`
	Query           string   `json:"query"`
	Tags            []string `json:"tags"`
	AgentID         string   `json:"agent_id"`
	Limit           int      `json:"limit"`
	HivySessionID   string   `json:"_hivy_session_id"`
	HivyActorUserID string   `json:"_hivy_actor_user_id"`
}

// orgMemoriesEnabled reports whether the calling agent may use the privileged
// org_memories supervisor tool. The default Hivy agent gets it automatically.
// Any other agent must explicitly allow-list it in McpToolFilter.Allow. This is
// an intentional exception to the normal "nil filter = all MCP tools allowed"
// rule: this tool is never granted implicitly.
func orgMemoriesEnabled(agent *model.Agent) bool {
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
		if strings.TrimSpace(allowed) == orgMemoriesToolName {
			return true
		}
	}
	return false
}

func (s *Service) loadOrgAgent(ctx context.Context, orgID, agentID uuid.UUID) (*model.Agent, error) {
	var agent model.Agent
	if err := s.cfg.DB.WithContext(ctx).
		Where("id = ? AND org_id = ? AND status <> ?", agentID, orgID, "archived").
		First(&agent).Error; err != nil {
		return nil, err
	}
	return &agent, nil
}

func registerOrgMemoriesTool(server *mcp.Server, service *Service, token *model.Token) {
	server.AddTool(&mcp.Tool{
		Name:        orgMemoriesToolName,
		Description: orgMemoriesDescription,
		InputSchema: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"action": map[string]any{
					"type":        "string",
					"enum":        []string{"search", "overview"},
					"description": "search runs a semantic search across all agents' org memories; overview returns aggregate counts only.",
				},
				"query": map[string]any{
					"type":        "string",
					"description": "Required for action search. Short semantic search phrase, max 6 words and 40 characters. Example: railway services inventory.",
					"maxLength":   memoryToolQueryMaxChars,
				},
				"agent_id": map[string]any{
					"type":        "string",
					"description": "Optional for action search. UUID of an agent in this org; narrows results to that agent's private org memories.",
				},
				"tags": memoryTagsSchema("Optional for action search. Exact filters using lowercase kebab-case slugs such as service-discovery or billing."),
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
		var args orgMemoriesArgs
		if err := decodeMemoryToolArgs(req, &args); err != nil {
			return memoryToolError(err.Error()), nil
		}
		// Reading every agent's org memories is an org-wide privilege: require the
		// acting human to be an org admin/owner. Automated runs (no actor) keep the
		// existing agent-gated behavior.
		if errResult := service.requireOrgManagerActor(ctx, token.OrgID, args.HivyActorUserID); errResult != nil {
			return errResult, nil
		}
		switch strings.ToLower(strings.TrimSpace(args.Action)) {
		case "search":
			return handleOrgMemoriesSearch(ctx, service, token, args)
		case "overview":
			return handleOrgMemoriesOverview(ctx, service, token)
		default:
			return memoryToolError("action must be search or overview"), nil
		}
	})
}
