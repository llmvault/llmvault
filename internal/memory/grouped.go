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

// GroupedAgent is one agent's slice of memories in the grouped view.
type GroupedAgent struct {
	AgentID   uuid.UUID
	AgentName string
	Memories  []model.AgentMemory
	HasMore   bool
	Total     int
	// NextCursor pages this agent past the returned slice (empty when !HasMore).
	NextCursor string
}

// GroupedByAgent returns every agent's newest memories in one query.
func (s *Service) GroupedByAgent(ctx context.Context, orgID uuid.UUID, perAgent int, vis Visibility) ([]GroupedAgent, error) {
	if s == nil || s.cfg.DB == nil {
		return nil, fmt.Errorf("memory service is not configured")
	}
	if orgID == uuid.Nil {
		return nil, fmt.Errorf("org_id is required")
	}
	if perAgent <= 0 || perAgent > 50 {
		perAgent = 10
	}
	args := []any{orgID, perAgent}
	var rows []groupedRow
	query := `
SELECT id, org_id, agent_id, content, memory_fingerprint, tags, metadata,
       embedding_model, embedding_status, embedding_revision, embedding_error,
       embedded_at, source_session_id, source_event_id, created_by_user_id,
       archived_at, created_at, updated_at, agent_name, agent_total
FROM (
  SELECT m.id, m.org_id, m.agent_id, m.content, m.memory_fingerprint, m.tags, m.metadata,
         m.embedding_model, m.embedding_status, m.embedding_revision, m.embedding_error,
         m.embedded_at, m.source_session_id, m.source_event_id, m.created_by_user_id,
         m.archived_at, m.created_at, m.updated_at,
         COALESCE(a.name, '') AS agent_name,
         row_number() OVER (PARTITION BY m.agent_id ORDER BY m.created_at DESC, m.id DESC) AS rn,
         count(*) OVER (PARTITION BY m.agent_id) AS agent_total
  FROM agent_memories m
  JOIN agents a ON a.id = m.agent_id
  WHERE m.org_id = ? AND m.archived_at IS NULL
) t
WHERE t.rn <= ?
ORDER BY agent_name, t.created_at DESC, t.id DESC`
	if err := s.cfg.DB.WithContext(ctx).Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("group memories by agent: %w", err)
	}

	groups := make([]GroupedAgent, 0)
	index := map[string]int{}
	for _, row := range rows {
		key := row.AgentID.String()
		pos, ok := index[key]
		if !ok {
			pos = len(groups)
			index[key] = pos
			groups = append(groups, GroupedAgent{
				AgentID:   row.AgentID,
				AgentName: row.AgentName,
				HasMore:   row.AgentTotal > perAgent,
				Total:     row.AgentTotal,
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

// ListAgentPage returns one agent's memories after cursor, newest first, for
// the "load more" control. An empty cursor starts from the newest.
func (s *Service) ListAgentPage(ctx context.Context, orgID uuid.UUID, scope AgentScope, cursor string, limit int) ([]model.AgentMemory, string, bool, error) {
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
		return nil, "", false, fmt.Errorf("list agent memories: %w", err)
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
	AgentName  string `gorm:"column:agent_name"`
	AgentTotal int    `gorm:"column:agent_total"`
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
