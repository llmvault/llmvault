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

func handleManageSearch(ctx context.Context, service *Service, token *model.Token, args manageMemoriesArgs) (*mcp.CallToolResult, error) {
	query, err := normalizeMemoryToolSearchQuery(args.Query)
	if err != nil {
		return memoryToolError(err.Error()), nil
	}
	tags, err := normalizeMemoryToolTags(args.Tags)
	if err != nil {
		return memoryToolError(err.Error()), nil
	}
	channelID, explicit, err := resolveManageChannel(args.ChannelID)
	if err != nil {
		return memoryToolError(err.Error()), nil
	}
	limit := args.Limit
	if limit <= 0 {
		limit = memoryToolSearchLimit
	} else if limit > 50 {
		limit = 50
	}
	scope := ChannelScope{AllChannels: true}
	if explicit {
		scope = ChannelScope{ChannelID: channelID}
	}
	searchCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	hits, err := service.Search(searchCtx, SearchRequest{
		OrgID: token.OrgID,
		Scope: scope,
		Query: query,
		Tags:  tags,
		Limit: limit,
	})
	if err != nil {
		return memoryToolError("memory search failed: " + err.Error()), nil
	}
	names, err := service.channelNames(ctx, token.OrgID, hits)
	if err != nil {
		return memoryToolError("memory search failed: " + err.Error()), nil
	}
	out := map[string]any{
		"success": true,
		"query":   query,
		"results": manageSearchResponses(hits, names),
		"total":   len(hits),
	}
	if explicit {
		if channelID == nil {
			out["channel_id"] = "org"
		} else {
			out["channel_id"] = channelID.String()
		}
	}
	return memoryToolJSON(out)
}

func manageSearchResponses(hits []SearchHit, names map[uuid.UUID]string) []map[string]any {
	out := make([]map[string]any, 0, len(hits))
	for _, hit := range hits {
		similarity := hit.Similarity
		item := memoryToolMemoryResponse(hit.Memory, &similarity)
		if hit.Memory.ChannelID != nil {
			item["org_wide"] = false
			if name, ok := names[*hit.Memory.ChannelID]; ok {
				item["channel_name"] = name
			} else {
				item["channel_name"] = nil
			}
		} else {
			item["org_wide"] = true
			item["channel_name"] = nil
		}
		out = append(out, item)
	}
	return out
}

func (s *Service) channelNames(ctx context.Context, orgID uuid.UUID, hits []SearchHit) (map[uuid.UUID]string, error) {
	ids := make([]uuid.UUID, 0, len(hits))
	seen := map[uuid.UUID]bool{}
	for _, hit := range hits {
		if hit.Memory.ChannelID == nil || seen[*hit.Memory.ChannelID] {
			continue
		}
		seen[*hit.Memory.ChannelID] = true
		ids = append(ids, *hit.Memory.ChannelID)
	}
	names := map[uuid.UUID]string{}
	if len(ids) == 0 {
		return names, nil
	}
	var rows []struct {
		ID   uuid.UUID
		Name string
	}
	if err := s.cfg.DB.WithContext(ctx).Model(&model.Channel{}).
		Select("id, name").
		Where("org_id = ? AND id IN ?", orgID, ids).
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("load channel names: %w", err)
	}
	for _, row := range rows {
		names[row.ID] = row.Name
	}
	return names, nil
}

func handleManageForget(ctx context.Context, service *Service, token *model.Token, args manageMemoriesArgs) (*mcp.CallToolResult, error) {
	memoryID, err := uuid.Parse(strings.TrimSpace(args.MemoryID))
	if err != nil || memoryID == uuid.Nil {
		return memoryToolError("memory_id must be a valid UUID"), nil
	}
	if err := service.Archive(ctx, ArchiveRequest{OrgID: token.OrgID, ID: memoryID}); err != nil {
		return memoryToolError("forget failed: " + memoryToolLoadMessage(err)), nil
	}
	return memoryToolJSON(map[string]any{
		"success":   true,
		"memory_id": memoryID.String(),
		"status":    "archived",
	})
}

func handleManageRetain(ctx context.Context, service *Service, token *model.Token, args manageMemoriesArgs) (*mcp.CallToolResult, error) {
	tags, err := normalizeMemoryToolTags(args.Tags)
	if err != nil {
		return memoryToolError(err.Error()), nil
	}
	channelID, _, err := resolveManageChannel(args.ChannelID)
	if err != nil {
		return memoryToolError(err.Error()), nil
	}
	createCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	mem, err := service.Create(createCtx, CreateRequest{
		OrgID:     token.OrgID,
		ChannelID: channelID,
		Content:   args.Content,
		Tags:      tags,
		Metadata:  args.Metadata,
	})
	if err != nil {
		return memoryToolError("retain failed: " + err.Error()), nil
	}
	return memoryToolJSON(map[string]any{
		"success": true,
		"memory":  memoryToolMemoryResponse(*mem, nil),
		"note":    "Embedding is queued asynchronously; semantic search becomes available after embedding_status is ready.",
	})
}

func handleManageOverview(ctx context.Context, service *Service, token *model.Token) (*mcp.CallToolResult, error) {
	db := service.cfg.DB.WithContext(ctx)
	var total, orgWide int64
	if err := db.Model(&model.AgentMemory{}).
		Where("org_id = ? AND archived_at IS NULL", token.OrgID).
		Count(&total).Error; err != nil {
		return memoryToolError("memory overview failed: " + err.Error()), nil
	}
	if err := db.Model(&model.AgentMemory{}).
		Where("org_id = ? AND archived_at IS NULL AND channel_id IS NULL", token.OrgID).
		Count(&orgWide).Error; err != nil {
		return memoryToolError("memory overview failed: " + err.Error()), nil
	}
	var channelRows []struct {
		ChannelID uuid.UUID
		Name      *string
		Count     int64
	}
	if err := db.Raw(`
SELECT m.channel_id AS channel_id, c.name AS name, COUNT(*) AS count
FROM agent_memories m
LEFT JOIN channels c ON c.id = m.channel_id AND c.org_id = m.org_id
WHERE m.org_id = ? AND m.archived_at IS NULL AND m.channel_id IS NOT NULL
GROUP BY m.channel_id, c.name
ORDER BY count DESC, c.name ASC NULLS LAST`, token.OrgID).
		Scan(&channelRows).Error; err != nil {
		return memoryToolError("memory overview failed: " + err.Error()), nil
	}
	channels := make([]map[string]any, 0, len(channelRows))
	for _, row := range channelRows {
		item := map[string]any{"channel_id": row.ChannelID.String(), "count": row.Count}
		if row.Name != nil {
			item["name"] = *row.Name
		} else {
			item["name"] = nil
		}
		channels = append(channels, item)
	}
	var tagRows []struct {
		Tag   string
		Count int64
	}
	if err := db.Raw(`
SELECT tag, COUNT(*) AS count
FROM agent_memories m, unnest(m.tags) AS tag
WHERE m.org_id = ? AND m.archived_at IS NULL
GROUP BY tag
ORDER BY count DESC, tag ASC
LIMIT ?`, token.OrgID, manageMemoriesTopTagsLimit).
		Scan(&tagRows).Error; err != nil {
		return memoryToolError("memory overview failed: " + err.Error()), nil
	}
	topTags := make([]map[string]any, 0, len(tagRows))
	for _, row := range tagRows {
		topTags = append(topTags, map[string]any{"tag": row.Tag, "count": row.Count})
	}
	return memoryToolJSON(map[string]any{
		"success":        true,
		"total":          total,
		"org_wide_count": orgWide,
		"channels":       channels,
		"top_tags":       topTags,
	})
}
