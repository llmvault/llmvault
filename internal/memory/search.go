package memory

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"

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
	switch strings.TrimSpace(req.Scope) {
	case model.AgentMemoryScopeOrg:
		where = append(where, "scope = ?")
		args = append(args, model.AgentMemoryScopeOrg)
	case model.AgentMemoryScopeUser:
		if req.UserID == nil || *req.UserID == uuid.Nil {
			return nil, nil
		}
		where = append(where, "scope = ? AND user_id = ?")
		args = append(args, model.AgentMemoryScopeUser, *req.UserID)
	default:
		if req.UserID != nil && *req.UserID != uuid.Nil {
			where = append(where, "(scope = 'org' OR (scope = 'user' AND user_id = ?))")
			args = append(args, *req.UserID)
		} else {
			where = append(where, "scope = 'org'")
		}
	}
	switch strings.TrimSpace(req.AgentVisibility) {
	case AgentVisibilityAllAgents:
		where = append(where, "agent_id IS NULL")
	case AgentVisibilityThisAgent:
		if req.AgentID == uuid.Nil {
			return nil, nil
		}
		where = append(where, "agent_id = ?")
		args = append(args, req.AgentID)
	default:
		if req.AgentID != uuid.Nil {
			where = append(where, "(agent_id IS NULL OR agent_id = ?)")
			args = append(args, req.AgentID)
		}
	}
	if tags := NormalizeTags(req.Tags); len(tags) > 0 {
		where = append(where, "tags && ?")
		args = append(args, pq.StringArray(tags))
	}
	args = append([]any{vectorLiteral(vector)}, args...)
	args = append(args, limit)

	var rows []searchRow
	query := `
SELECT id, org_id, agent_id, user_id, scope, content, tags, metadata,
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

func (s *Service) SearchForTurn(ctx context.Context, req SearchRequest) (TurnMemories, error) {
	limit := maxPositive(req.Limit, 5)
	vector := req.QueryVector
	if len(vector) == 0 {
		var err error
		vector, err = s.EmbedQuery(ctx, req.Query)
		if err != nil {
			return TurnMemories{}, err
		}
	}
	orgReq := req
	orgReq.Scope = model.AgentMemoryScopeOrg
	orgReq.UserID = nil
	orgReq.QueryVector = vector
	orgReq.Limit = limit
	orgHits, err := s.Search(ctx, orgReq)
	if err != nil {
		return TurnMemories{}, err
	}
	out := TurnMemories{Org: orgHits}
	if req.UserID == nil || *req.UserID == uuid.Nil {
		return out, nil
	}
	userReq := req
	userReq.Scope = model.AgentMemoryScopeUser
	userReq.QueryVector = vector
	userReq.Limit = limit
	userHits, err := s.Search(ctx, userReq)
	if err != nil {
		return TurnMemories{}, err
	}
	out.User = userHits
	return out, nil
}

func applyAccessibleScope(q *gorm.DB, userID *uuid.UUID) *gorm.DB {
	if userID != nil && *userID != uuid.Nil {
		return q.Where("(scope = ? OR (scope = ? AND user_id = ?))",
			model.AgentMemoryScopeOrg, model.AgentMemoryScopeUser, *userID)
	}
	return q.Where("scope = ?", model.AgentMemoryScopeOrg)
}

func maxPositive(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

type searchRow struct {
	model.AgentMemory
	Similarity float64 `gorm:"column:similarity"`
}
