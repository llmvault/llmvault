package memory

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/usehivy/hivy/internal/channelagents"
	"github.com/usehivy/hivy/internal/model"
)

func (s *Service) Search(ctx context.Context, req SearchRequest) ([]SearchHit, error) {
	if s == nil || s.cfg.DB == nil {
		return nil, fmt.Errorf("memory service is not configured")
	}
	if req.OrgID == uuid.Nil {
		return nil, fmt.Errorf("org_id is required")
	}
	vector := req.QueryVector
	if len(vector) == 0 {
		var err error
		vector, err = s.EmbedQuery(ctx, req.Query)
		if err != nil {
			return nil, err
		}
	}
	if err := validateVector(vector, s.embeddingDim()); err != nil {
		return nil, err
	}
	limit := req.Limit
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	where := []string{
		"org_id = ?",
		"archived_at IS NULL",
		"embedding_status = ?",
		"embedding IS NOT NULL",
		"embedding_model = ?",
	}
	args := []any{req.OrgID, model.AgentMemoryEmbeddingReady, s.embeddingModel()}
	if clause, scopeArgs := req.Scope.whereSQL(); clause != "" {
		where = append(where, clause)
		args = append(args, scopeArgs...)
	}
	if req.Visibility.Restrict {
		where = append(where, "(channel_id IS NULL OR channel_id IN (?))")
		args = append(args, channelagents.VisibleChannelIDsSubquery(s.cfg.DB, req.OrgID, req.Visibility.UserID))
	}
	if tags := NormalizeTags(req.Tags); len(tags) > 0 {
		where = append(where, "tags && ?")
		args = append(args, pq.StringArray(tags))
	}
	args = append([]any{vectorLiteral(vector)}, args...)
	args = append(args, limit)

	var rows []searchRow
	query := `
SELECT id, org_id, channel_id, content, memory_fingerprint, tags, metadata,
       embedding_model, embedding_status, embedding_revision, embedding_error,
       embedded_at, source_session_id, source_event_id, created_by_user_id,
       archived_at, created_at, updated_at,
       1 - (embedding <=> ?::vector) AS similarity
FROM agent_memories
WHERE ` + strings.Join(where, " AND ") + `
ORDER BY embedding <=> ?::vector
LIMIT ?`
	args = append(args[:len(args)-1], args[0], args[len(args)-1])
	if err := s.cfg.DB.WithContext(ctx).Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("search memories: %w", err)
	}
	hits := make([]SearchHit, len(rows))
	for i := range rows {
		hits[i] = SearchHit{Memory: rows[i].AgentMemory, Similarity: rows[i].Similarity}
	}
	return hits, nil
}

type searchRow struct {
	model.AgentMemory
	Similarity float64 `gorm:"column:similarity"`
}
