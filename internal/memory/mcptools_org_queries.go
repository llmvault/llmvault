package memory

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/usehivy/hivy/internal/model"
)

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
