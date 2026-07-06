package memory

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/usehivy/hivy/internal/model"
)

// observationLiveSQL filters to observations that should still be recalled:
// not archived (supersession and expiry both archive) and not past expiry.
const observationLiveSQL = "archived_at IS NULL AND (expires_at IS NULL OR expires_at > now())"

// TopObservations returns the channel's strongest live observations ordered by
// proof_count then recency — the synchronous fallback when a channel has no
// precomputed memory digest yet. One indexed query; no embeddings. Before the
// observations migration lands the table reads as empty.
func (s *Service) TopObservations(ctx context.Context, orgID uuid.UUID, scope ChannelScope, limit int) ([]model.AgentObservation, error) {
	if s == nil || s.cfg.DB == nil {
		return nil, fmt.Errorf("memory service is not configured")
	}
	if orgID == uuid.Nil {
		return nil, fmt.Errorf("org_id is required")
	}
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	q := s.cfg.DB.WithContext(ctx).
		Where("org_id = ?", orgID).
		Where(observationLiveSQL).
		Order("proof_count DESC, last_mentioned_at DESC, id DESC").
		Limit(limit)
	if clause, args := scope.whereSQL(); clause != "" {
		q = q.Where(clause, args...)
	}
	var rows []model.AgentObservation
	if err := q.Find(&rows).Error; err != nil {
		if isUndefinedTable(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("list top observations: %w", err)
	}
	return rows, nil
}

// SearchObservations runs the agent-facing semantic search over consolidated
// observations with the same ChannelScope semantics as Search over facts (the
// worker-side vector lookup is SimilarObservations). Tags (lowercase slugs)
// are matched case-insensitively against observation entities. Before the
// observations migration lands the table reads as empty.
func (s *Service) SearchObservations(ctx context.Context, req SearchRequest) ([]ObservationHit, error) {
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
		observationLiveSQL,
		"embedding_status = ?",
		"embedding IS NOT NULL",
		"embedding_model = ?",
	}
	args := []any{req.OrgID, model.AgentMemoryEmbeddingReady, s.embeddingModel()}
	if clause, scopeArgs := req.Scope.whereSQL(); clause != "" {
		where = append(where, clause)
		args = append(args, scopeArgs...)
	}
	if tags := NormalizeTags(req.Tags); len(tags) > 0 {
		where = append(where, "EXISTS (SELECT 1 FROM unnest(entities) AS entity WHERE lower(entity) = ANY(?))")
		args = append(args, pq.StringArray(tags))
	}
	literal := vectorLiteral(vector)
	args = append([]any{literal}, args...)
	args = append(args, literal, limit)

	var rows []observationSearchRow
	query := `
SELECT id, org_id, channel_id, content, kind, entities, proof_count, source_fact_ids,
       occurred_start, occurred_end, last_mentioned_at, expires_at, superseded_by,
       archived_at, human_verified, embedding_model, embedding_status,
       embedding_revision, embedding_error, embedded_at, metadata, created_at, updated_at,
       1 - (embedding <=> ?::vector) AS similarity
FROM agent_observations
WHERE ` + strings.Join(where, " AND ") + `
ORDER BY embedding <=> ?::vector
LIMIT ?`
	if err := s.cfg.DB.WithContext(ctx).Raw(query, args...).Scan(&rows).Error; err != nil {
		if isUndefinedTable(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("search observations: %w", err)
	}
	hits := make([]ObservationHit, len(rows))
	for i := range rows {
		hits[i] = ObservationHit{Observation: rows[i].AgentObservation, Similarity: rows[i].Similarity}
	}
	return hits, nil
}

// isUndefinedTable reports whether err is Postgres undefined_table (42P01):
// reads treat not-yet-migrated memory tables as empty so rollout ordering
// (deploy before/with the migration) never breaks recall.
func isUndefinedTable(err error) bool {
	return err != nil && strings.Contains(err.Error(), "SQLSTATE 42P01")
}
