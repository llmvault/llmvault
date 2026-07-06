package memory

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

// GroupedChannel is one channel's slice of memories in the grouped view: the
// newest perChannel memories plus a hasMore signal and the channel's full total.
type GroupedChannel struct {
	ChannelID   *uuid.UUID
	ChannelName string
	Memories    []model.AgentMemory
	HasMore     bool
	Total       int
	// NextCursor pages this channel past the returned slice (empty when !HasMore).
	NextCursor string
}

// GroupedByChannel returns every channel's newest perChannel memories in one
// query (a per-partition window), so the settings view renders all channels at
// once without N round-trips. Global (channel-less) memories come first, then
// channels by name. Use ListChannelPage to page past perChannel for one channel.
func (s *Service) GroupedByChannel(ctx context.Context, orgID uuid.UUID, perChannel int) ([]GroupedChannel, error) {
	if s == nil || s.cfg.DB == nil {
		return nil, fmt.Errorf("memory service is not configured")
	}
	if orgID == uuid.Nil {
		return nil, fmt.Errorf("org_id is required")
	}
	if perChannel <= 0 || perChannel > 50 {
		perChannel = 10
	}
	var rows []groupedRow
	query := `
SELECT id, org_id, channel_id, content, memory_fingerprint, tags, metadata,
       embedding_model, embedding_status, embedding_revision, embedding_error,
       embedded_at, source_session_id, source_event_id, created_by_user_id,
       archived_at, created_at, updated_at, channel_name, channel_total
FROM (
  SELECT m.id, m.org_id, m.channel_id, m.content, m.memory_fingerprint, m.tags, m.metadata,
         m.embedding_model, m.embedding_status, m.embedding_revision, m.embedding_error,
         m.embedded_at, m.source_session_id, m.source_event_id, m.created_by_user_id,
         m.archived_at, m.created_at, m.updated_at,
         COALESCE(c.name, '') AS channel_name,
         row_number() OVER (PARTITION BY m.channel_id ORDER BY m.created_at DESC, m.id DESC) AS rn,
         count(*) OVER (PARTITION BY m.channel_id) AS channel_total
  FROM agent_memories m
  LEFT JOIN channels c ON c.id = m.channel_id
  WHERE m.org_id = ? AND m.archived_at IS NULL
) t
WHERE t.rn <= ?
ORDER BY (t.channel_id IS NOT NULL), channel_name, t.created_at DESC, t.id DESC`
	if err := s.cfg.DB.WithContext(ctx).Raw(query, orgID, perChannel).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("group memories by channel: %w", err)
	}

	groups := make([]GroupedChannel, 0)
	index := map[string]int{}
	for _, row := range rows {
		key := "global"
		if row.ChannelID != nil {
			key = row.ChannelID.String()
		}
		pos, ok := index[key]
		if !ok {
			pos = len(groups)
			index[key] = pos
			groups = append(groups, GroupedChannel{
				ChannelID:   row.ChannelID,
				ChannelName: row.ChannelName,
				HasMore:     row.ChannelTotal > perChannel,
				Total:       row.ChannelTotal,
			})
		}
		groups[pos].Memories = append(groups[pos].Memories, row.AgentMemory)
	}
	for i := range groups {
		if !groups[i].HasMore || len(groups[i].Memories) == 0 {
			continue
		}
		last := groups[i].Memories[len(groups[i].Memories)-1]
		groups[i].NextCursor = encodeMemoryCursor(last.CreatedAt, last.ID)
	}
	return groups, nil
}

// ListChannelPage returns one channel's memories after cursor, newest first, for
// the "load more" control. An empty cursor starts from the newest.
func (s *Service) ListChannelPage(ctx context.Context, orgID uuid.UUID, scope ChannelScope, cursor string, limit int) ([]model.AgentMemory, string, bool, error) {
	if s == nil || s.cfg.DB == nil {
		return nil, "", false, fmt.Errorf("memory service is not configured")
	}
	if orgID == uuid.Nil {
		return nil, "", false, fmt.Errorf("org_id is required")
	}
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	q := s.cfg.DB.WithContext(ctx).Model(&model.AgentMemory{}).
		Where("org_id = ? AND archived_at IS NULL", orgID)
	if clause, args := scope.whereSQL(); clause != "" {
		q = q.Where(clause, args...)
	}
	if cursor != "" {
		at, id, err := decodeMemoryCursor(cursor)
		if err != nil {
			return nil, "", false, err
		}
		q = q.Where("(created_at, id) < (?, ?)", at, id)
	}
	var rows []model.AgentMemory
	if err := q.Order("created_at DESC, id DESC").Limit(limit + 1).Find(&rows).Error; err != nil {
		return nil, "", false, fmt.Errorf("list channel memories: %w", err)
	}
	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}
	next := ""
	if hasMore && len(rows) > 0 {
		last := rows[len(rows)-1]
		next = encodeMemoryCursor(last.CreatedAt, last.ID)
	}
	return rows, next, hasMore, nil
}

type groupedRow struct {
	model.AgentMemory
	ChannelName  string `gorm:"column:channel_name"`
	ChannelTotal int    `gorm:"column:channel_total"`
}

// Memory cursors encode the ordering key (created_at, id) so paging survives
// inserts between requests.
func encodeMemoryCursor(createdAt time.Time, id uuid.UUID) string {
	raw := createdAt.UTC().Format(time.RFC3339Nano) + "|" + id.String()
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeMemoryCursor(cursor string) (time.Time, uuid.UUID, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, uuid.Nil, fmt.Errorf("invalid cursor")
	}
	parts := strings.SplitN(string(raw), "|", 2)
	if len(parts) != 2 {
		return time.Time{}, uuid.Nil, fmt.Errorf("invalid cursor")
	}
	at, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return time.Time{}, uuid.Nil, fmt.Errorf("invalid cursor")
	}
	id, err := uuid.Parse(parts[1])
	if err != nil {
		return time.Time{}, uuid.Nil, fmt.Errorf("invalid cursor")
	}
	return at, id, nil
}
