package memory

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

// MarkObservationEmbeddingReady stores the embedding vector when the revision
// still matches (stale revisions are dropped, mirroring agent_memories).
func (s *Service) MarkObservationEmbeddingReady(ctx context.Context, id uuid.UUID, revision int, vector []float32) (bool, error) {
	if err := validateVector(vector, s.embeddingDim()); err != nil {
		return false, err
	}
	res := s.cfg.DB.WithContext(ctx).Model(&model.AgentObservation{}).
		Where("id = ? AND embedding_revision = ? AND archived_at IS NULL", id, revision).
		Updates(map[string]any{
			"embedding":        gorm.Expr("?::vector", vectorLiteral(vector)),
			"embedding_model":  s.embeddingModel(),
			"embedding_status": model.AgentMemoryEmbeddingReady,
			"embedding_error":  "",
			"embedded_at":      time.Now(),
			"updated_at":       time.Now(),
		})
	if res.Error != nil {
		return false, fmt.Errorf("mark observation embedding ready: %w", res.Error)
	}
	return res.RowsAffected > 0, nil
}

// MarkObservationEmbeddingFailed records an embedding failure for a revision.
func (s *Service) MarkObservationEmbeddingFailed(ctx context.Context, id uuid.UUID, revision int, cause error) error {
	msg := ""
	if cause != nil {
		msg = strings.TrimSpace(cause.Error())
	}
	if len(msg) > 500 {
		msg = msg[:500]
	}
	return s.cfg.DB.WithContext(ctx).Model(&model.AgentObservation{}).
		Where("id = ? AND embedding_revision = ? AND archived_at IS NULL", id, revision).
		Updates(map[string]any{
			"embedding_status": model.AgentMemoryEmbeddingFailed,
			"embedding_error":  msg,
			"updated_at":       time.Now(),
		}).Error
}

// SimilarObservationsRequest selects the vector-nearest non-archived
// observations. ChannelID set + IncludeOrgWide folds org-wide rows in
// (consolidation lookup); ChannelID set alone = that channel only and
// ChannelID nil = org-wide only (semantic-dedup same-scope probes).
type SimilarObservationsRequest struct {
	OrgID          uuid.UUID
	ChannelID      *uuid.UUID
	IncludeOrgWide bool
	Vector         []float32
	Limit          int
	ExcludeID      *uuid.UUID
}

// ObservationHit pairs an observation with its cosine similarity.
type ObservationHit struct {
	Observation model.AgentObservation
	Similarity  float64
}

// SimilarObservations runs a pgvector cosine top-K over agent_observations.
func (s *Service) SimilarObservations(ctx context.Context, req SimilarObservationsRequest) ([]ObservationHit, error) {
	if s == nil || s.cfg.DB == nil {
		return nil, fmt.Errorf("memory service is not configured")
	}
	if req.OrgID == uuid.Nil {
		return nil, fmt.Errorf("org_id is required")
	}
	if err := validateVector(req.Vector, s.embeddingDim()); err != nil {
		return nil, err
	}
	limit := req.Limit
	if limit <= 0 || limit > 50 {
		limit = 5
	}
	where := []string{
		"org_id = ?",
		"archived_at IS NULL",
		"embedding_status = ?",
		"embedding IS NOT NULL",
		"embedding_model = ?",
	}
	args := []any{req.OrgID, model.AgentMemoryEmbeddingReady, s.embeddingModel()}
	switch {
	case req.ChannelID != nil && req.IncludeOrgWide:
		where = append(where, "(channel_id = ? OR channel_id IS NULL)")
		args = append(args, *req.ChannelID)
	case req.ChannelID != nil:
		where = append(where, "channel_id = ?")
		args = append(args, *req.ChannelID)
	default:
		where = append(where, "channel_id IS NULL")
	}
	if req.ExcludeID != nil && *req.ExcludeID != uuid.Nil {
		where = append(where, "id <> ?")
		args = append(args, *req.ExcludeID)
	}
	vector := vectorLiteral(req.Vector)
	args = append([]any{vector}, args...)
	args = append(args, vector, limit)

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
		return nil, fmt.Errorf("search observations: %w", err)
	}
	hits := make([]ObservationHit, len(rows))
	for i := range rows {
		hits[i] = ObservationHit{Observation: rows[i].AgentObservation, Similarity: rows[i].Similarity}
	}
	return hits, nil
}

type observationSearchRow struct {
	model.AgentObservation
	Similarity float64 `gorm:"column:similarity"`
}

// consolidationFactSources are the agent_memories sources that flow into
// consolidation: reflection-extracted facts and legacy memories agents stored
// via the (since removed) retain_memory MCP tool. The tool is gone — agents
// are read-only on memory — but historical "mcp_memory_tool" facts still
// exist and must keep flowing into the observations layer: it is the only
// layer recall injects and search reads, so a source left out of this list is
// written but never recalled.
var consolidationFactSources = []string{"reflection", "mcp_memory_tool"}

// ListUnconsolidatedFacts returns the oldest facts in a channel that
// consolidation has not folded into observations yet (reflection-extracted
// and agent-retained).
func (s *Service) ListUnconsolidatedFacts(ctx context.Context, orgID, channelID uuid.UUID, limit int) ([]model.AgentMemory, error) {
	if s == nil || s.cfg.DB == nil {
		return nil, fmt.Errorf("memory service is not configured")
	}
	if limit <= 0 {
		limit = 30
	}
	var rows []model.AgentMemory
	err := s.cfg.DB.WithContext(ctx).
		Where("org_id = ? AND channel_id = ? AND archived_at IS NULL AND consolidated_at IS NULL", orgID, channelID).
		Where("metadata->>'source' IN ?", consolidationFactSources).
		Order("created_at ASC, id ASC").
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list unconsolidated facts: %w", err)
	}
	return rows, nil
}

// MarkFactsConsolidated stamps consolidated_at on processed facts.
func (s *Service) MarkFactsConsolidated(ctx context.Context, orgID uuid.UUID, ids []uuid.UUID, at time.Time) error {
	if s == nil || s.cfg.DB == nil {
		return fmt.Errorf("memory service is not configured")
	}
	if len(ids) == 0 {
		return nil
	}
	return s.cfg.DB.WithContext(ctx).Model(&model.AgentMemory{}).
		Where("org_id = ? AND id IN ?", orgID, ids).
		Updates(map[string]any{"consolidated_at": at, "updated_at": at}).Error
}

// IsSuppressed reports whether content matches a suppression fingerprint for
// the channel or the org-wide scope.
func (s *Service) IsSuppressed(ctx context.Context, orgID uuid.UUID, channelID *uuid.UUID, content string) (bool, error) {
	if s == nil || s.cfg.DB == nil {
		return false, fmt.Errorf("memory service is not configured")
	}
	fingerprint := SuppressionFingerprint(content)
	q := s.cfg.DB.WithContext(ctx).Model(&model.MemorySuppression{}).
		Where("org_id = ? AND content_fingerprint = ?", orgID, fingerprint)
	if channelID != nil && *channelID != uuid.Nil {
		q = q.Where("(channel_id = ? OR channel_id IS NULL)", *channelID)
	} else {
		q = q.Where("channel_id IS NULL")
	}
	var count int64
	if err := q.Count(&count).Error; err != nil {
		return false, fmt.Errorf("check memory suppression: %w", err)
	}
	return count > 0, nil
}

// ExpireObservations archives every observation whose expires_at has passed
// and returns the archived rows so callers can refresh affected digests.
func (s *Service) ExpireObservations(ctx context.Context, now time.Time) ([]model.AgentObservation, error) {
	if s == nil || s.cfg.DB == nil {
		return nil, fmt.Errorf("memory service is not configured")
	}
	var rows []model.AgentObservation
	if err := s.cfg.DB.WithContext(ctx).
		Where("archived_at IS NULL AND expires_at IS NOT NULL AND expires_at < ?", now).
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("load expired observations: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	ids := make([]uuid.UUID, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	if err := s.cfg.DB.WithContext(ctx).Model(&model.AgentObservation{}).
		Where("id IN ? AND archived_at IS NULL", ids).
		Updates(map[string]any{"archived_at": now, "updated_at": now}).Error; err != nil {
		return nil, fmt.Errorf("archive expired observations: %w", err)
	}
	return rows, nil
}

// OrgChannel identifies one channel-scoped consolidation unit.
type OrgChannel struct {
	OrgID     uuid.UUID
	ChannelID uuid.UUID
}

// ChannelsWithUnconsolidatedFacts finds channels holding unprocessed facts
// (reflection-extracted or agent-retained) — the stranded-facts sweep source.
func (s *Service) ChannelsWithUnconsolidatedFacts(ctx context.Context, limit int) ([]OrgChannel, error) {
	if s == nil || s.cfg.DB == nil {
		return nil, fmt.Errorf("memory service is not configured")
	}
	if limit <= 0 {
		limit = 200
	}
	var rows []OrgChannel
	err := s.cfg.DB.WithContext(ctx).Raw(`
SELECT DISTINCT org_id, channel_id
FROM agent_memories
WHERE archived_at IS NULL
	AND consolidated_at IS NULL
	AND channel_id IS NOT NULL
	AND metadata->>'source' = ANY(?)
LIMIT ?`, pq.StringArray(consolidationFactSources), limit).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("scan unconsolidated channels: %w", err)
	}
	return rows, nil
}

// EmbedContents embeds a batch of texts with the memory embedding model.
func (s *Service) EmbedContents(ctx context.Context, contents []string) ([][]float32, error) {
	if len(contents) == 0 {
		return nil, nil
	}
	emb, err := s.embedder(ctx)
	if err != nil {
		return nil, err
	}
	vectors, err := emb.Embed(ctx, contents)
	if err != nil {
		return nil, err
	}
	if len(vectors) != len(contents) {
		return nil, fmt.Errorf("embed returned %d vectors for %d inputs", len(vectors), len(contents))
	}
	for _, vector := range vectors {
		if err := validateVector(vector, s.embeddingDim()); err != nil {
			return nil, err
		}
	}
	return vectors, nil
}
