package memory

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/usehivy/hivy/internal/model"
)

func handleManageSearch(ctx context.Context, service *Service, token *model.Token, agentID uuid.UUID, args manageMemoriesArgs) (*mcp.CallToolResult, error) {
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
	searchCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	searchReq := SearchRequest{OrgID: token.OrgID, Scope: AgentScope{AgentID: agentID}, Query: query, Tags: tags, Limit: limit}
	out := map[string]any{"success": true, "query": query, "agent_id": agentID.String()}
	if args.IncludeFacts {
		hits, err := service.Search(searchCtx, searchReq)
		if err != nil {
			return memoryToolError("memory search failed: " + err.Error()), nil
		}
		out["layer"] = memoryLayerFacts
		out["results"] = memorySearchResponses(hits)
		out["total"] = len(hits)
	} else {
		hits, err := service.SearchObservations(searchCtx, searchReq)
		if err != nil {
			return memoryToolError("memory search failed: " + err.Error()), nil
		}
		out["layer"] = memoryLayerObservations
		out["results"] = observationSearchResponses(hits)
		out["total"] = len(hits)
	}
	return memoryToolJSON(out)
}

func memorySearchResponses(hits []SearchHit) []map[string]any {
	out := make([]map[string]any, 0, len(hits))
	for _, hit := range hits {
		similarity := hit.Similarity
		out = append(out, memoryToolMemoryResponse(hit.Memory, &similarity))
	}
	return out
}

func observationSearchResponses(hits []ObservationHit) []map[string]any {
	out := make([]map[string]any, 0, len(hits))
	for _, hit := range hits {
		similarity := hit.Similarity
		out = append(out, observationToolResponse(hit.Observation, &similarity))
	}
	return out
}

func handleManageOverview(ctx context.Context, service *Service, token *model.Token, agentID uuid.UUID) (*mcp.CallToolResult, error) {
	db := service.cfg.DB.WithContext(ctx)
	var total int64
	if err := db.Model(&model.AgentMemory{}).
		Where("org_id = ? AND agent_id = ? AND archived_at IS NULL", token.OrgID, agentID).
		Count(&total).Error; err != nil {
		return memoryToolError("memory overview failed: " + err.Error()), nil
	}
	var tagRows []struct {
		Tag   string
		Count int64
	}
	if err := db.Raw(`
SELECT tag, COUNT(*) AS count
FROM agent_memories m, unnest(m.tags) AS tag
WHERE m.org_id = ? AND m.agent_id = ? AND m.archived_at IS NULL
GROUP BY tag
ORDER BY count DESC, tag ASC
LIMIT ?`, token.OrgID, agentID, manageMemoriesTopTagsLimit).Scan(&tagRows).Error; err != nil {
		return memoryToolError("memory overview failed: " + err.Error()), nil
	}
	topTags := make([]map[string]any, 0, len(tagRows))
	for _, row := range tagRows {
		topTags = append(topTags, map[string]any{"tag": row.Tag, "count": row.Count})
	}
	return memoryToolJSON(map[string]any{"success": true, "agent_id": agentID.String(), "total": total, "top_tags": topTags})
}
