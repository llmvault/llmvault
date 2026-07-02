package memory

import (
	"context"
	"fmt"
	"strings"
	"time"

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

func handleOrgMemoriesSearch(ctx context.Context, service *Service, token *model.Token, args orgMemoriesArgs) (*mcp.CallToolResult, error) {
	query, err := normalizeMemoryToolSearchQuery(args.Query)
	if err != nil {
		return memoryToolError(err.Error()), nil
	}
	tags, err := normalizeMemoryToolTags(args.Tags)
	if err != nil {
		return memoryToolError(err.Error()), nil
	}
	limit := args.Limit
	if limit <= 0 {
		limit = memoryToolSearchLimit
	} else if limit > 50 {
		limit = 50
	}
	// Org-scoped memories ONLY: shared (agent_id IS NULL) plus every agent's
	// bindings. User-scoped rows are a privacy boundary and are never searched.
	req := SearchRequest{
		OrgID: token.OrgID,
		Scope: model.AgentMemoryScopeOrg,
		Query: query,
		Tags:  tags,
		Limit: limit,
	}
	filterText := strings.TrimSpace(args.AgentID)
	if filterText != "" {
		filterAgentID, err := uuid.Parse(filterText)
		if err != nil || filterAgentID == uuid.Nil {
			return memoryToolError("agent_id must be a valid UUID"), nil
		}
		req.AgentVisibility = AgentVisibilityThisAgent
		req.AgentID = filterAgentID
	}
	searchCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	hits, err := service.Search(searchCtx, req)
	if err != nil {
		return memoryToolError("org memory search failed: " + err.Error()), nil
	}
	names, err := service.orgAgentNames(ctx, token.OrgID, hits)
	if err != nil {
		return memoryToolError("org memory search failed: " + err.Error()), nil
	}
	out := map[string]any{
		"success": true,
		"query":   query,
		"results": orgMemoriesSearchResponses(hits, names),
		"total":   len(hits),
	}
	if filterText != "" {
		out["agent_id"] = req.AgentID.String()
	}
	return memoryToolJSON(out)
}

func orgMemoriesSearchResponses(hits []SearchHit, names map[uuid.UUID]string) []map[string]any {
	out := make([]map[string]any, 0, len(hits))
	for _, hit := range hits {
		similarity := hit.Similarity
		item := memoryToolMemoryResponse(hit.Memory, &similarity)
		if hit.Memory.AgentID != nil {
			item["agent_id"] = hit.Memory.AgentID.String()
			item["shared"] = false
			if name, ok := names[*hit.Memory.AgentID]; ok {
				item["agent_name"] = name
			} else {
				item["agent_name"] = nil
			}
		} else {
			item["agent_id"] = nil
			item["agent_name"] = nil
			item["shared"] = true
		}
		out = append(out, item)
	}
	return out
}

func (s *Service) orgAgentNames(ctx context.Context, orgID uuid.UUID, hits []SearchHit) (map[uuid.UUID]string, error) {
	ids := make([]uuid.UUID, 0, len(hits))
	seen := map[uuid.UUID]bool{}
	for _, hit := range hits {
		if hit.Memory.AgentID == nil || seen[*hit.Memory.AgentID] {
			continue
		}
		seen[*hit.Memory.AgentID] = true
		ids = append(ids, *hit.Memory.AgentID)
	}
	names := map[uuid.UUID]string{}
	if len(ids) == 0 {
		return names, nil
	}
	var rows []struct {
		ID   uuid.UUID
		Name string
	}
	if err := s.cfg.DB.WithContext(ctx).Model(&model.Agent{}).
		Select("id, name").
		Where("org_id = ? AND id IN ?", orgID, ids).
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("load agent names: %w", err)
	}
	for _, row := range rows {
		names[row.ID] = row.Name
	}
	return names, nil
}

func handleOrgMemoriesOverview(ctx context.Context, service *Service, token *model.Token) (*mcp.CallToolResult, error) {
	db := service.cfg.DB.WithContext(ctx)
	var total, shared, userScoped int64
	if err := db.Model(&model.AgentMemory{}).
		Where("org_id = ? AND scope = ? AND archived_at IS NULL", token.OrgID, model.AgentMemoryScopeOrg).
		Count(&total).Error; err != nil {
		return memoryToolError("org memory overview failed: " + err.Error()), nil
	}
	if err := db.Model(&model.AgentMemory{}).
		Where("org_id = ? AND scope = ? AND archived_at IS NULL AND agent_id IS NULL", token.OrgID, model.AgentMemoryScopeOrg).
		Count(&shared).Error; err != nil {
		return memoryToolError("org memory overview failed: " + err.Error()), nil
	}
	// Aggregate count only: user-scoped memory content and per-user breakdowns
	// are never exposed through this tool.
	if err := db.Model(&model.AgentMemory{}).
		Where("org_id = ? AND scope = ? AND archived_at IS NULL", token.OrgID, model.AgentMemoryScopeUser).
		Count(&userScoped).Error; err != nil {
		return memoryToolError("org memory overview failed: " + err.Error()), nil
	}
	var agentRows []struct {
		AgentID uuid.UUID
		Name    *string
		Count   int64
	}
	if err := db.Raw(`
SELECT m.agent_id AS agent_id, a.name AS name, COUNT(*) AS count
FROM agent_memories m
LEFT JOIN agents a ON a.id = m.agent_id AND a.org_id = m.org_id
WHERE m.org_id = ? AND m.scope = ? AND m.archived_at IS NULL AND m.agent_id IS NOT NULL
GROUP BY m.agent_id, a.name
ORDER BY count DESC, a.name ASC NULLS LAST`, token.OrgID, model.AgentMemoryScopeOrg).
		Scan(&agentRows).Error; err != nil {
		return memoryToolError("org memory overview failed: " + err.Error()), nil
	}
	agents := make([]map[string]any, 0, len(agentRows))
	for _, row := range agentRows {
		item := map[string]any{
			"agent_id": row.AgentID.String(),
			"count":    row.Count,
		}
		if row.Name != nil {
			item["name"] = *row.Name
		} else {
			item["name"] = nil
		}
		agents = append(agents, item)
	}
	var tagRows []struct {
		Tag   string
		Count int64
	}
	if err := db.Raw(`
SELECT tag, COUNT(*) AS count
FROM agent_memories m, unnest(m.tags) AS tag
WHERE m.org_id = ? AND m.scope = ? AND m.archived_at IS NULL
GROUP BY tag
ORDER BY count DESC, tag ASC
LIMIT ?`, token.OrgID, model.AgentMemoryScopeOrg, orgMemoriesTopTagsLimit).
		Scan(&tagRows).Error; err != nil {
		return memoryToolError("org memory overview failed: " + err.Error()), nil
	}
	topTags := make([]map[string]any, 0, len(tagRows))
	for _, row := range tagRows {
		topTags = append(topTags, map[string]any{"tag": row.Tag, "count": row.Count})
	}
	return memoryToolJSON(map[string]any{
		"success":           true,
		"total":             total,
		"shared_count":      shared,
		"agents":            agents,
		"top_tags":          topTags,
		"user_scoped_count": userScoped,
	})
}
