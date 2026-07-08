package memory

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/usehivy/hivy/internal/model"
)

// observationLiveSQL filters to observations that should still be recalled:
// not archived (supersession and expiry both archive) and not past expiry.
const observationLiveSQL = "archived_at IS NULL AND (expires_at IS NULL OR expires_at > now())"

const (
	// SearchMinSimilarity drops weak vector matches before they reach the
	// agent. Below this cosine similarity a hit is noise for the asymmetric
	// instruct-embedding setup, and returning it invites the model to stretch
	// irrelevant memories into answers. An empty result is meaningful: nothing
	// relevant is stored. Conservative floor, not yet calibrated against the
	// production score distribution — tune using the similarity values returned
	// in search result payloads.
	SearchMinSimilarity = 0.25
	// searchStrengthWeight blends memory strength into agent-facing search
	// ordering: score = similarity * (1 + weight*ln(1+DigestScore)). Similarity
	// dominates; a fresh, repeatedly confirmed rule gets at most a ~25% edge,
	// enough to win near-ties against one-off findings but never to outrank a
	// clearly better semantic match.
	searchStrengthWeight = 0.1
	// searchCandidateFactor oversamples the SQL cosine top-K so the strength
	// blend can promote well-proven memories from just below the cut line.
	searchCandidateFactor = 3
)

// SearchScore orders agent-facing search hits: cosine similarity, mildly
// boosted by the same strength signal the channel digest uses (kind weight,
// proof count, recency decay via DigestScore).
func SearchScore(hit ObservationHit, now time.Time) float64 {
	return hit.Similarity * (1 + searchStrengthWeight*math.Log1p(DigestScore(hit.Observation, now)))
}

// filterAndRankSearchHits applies the relevance cutoff, the strength-blended
// ordering, and the final limit to cosine-ordered candidates (ties broken by
// recency then id for stability).
func filterAndRankSearchHits(hits []ObservationHit, now time.Time, limit int) []ObservationHit {
	kept := make([]ObservationHit, 0, len(hits))
	for _, hit := range hits {
		if hit.Similarity >= SearchMinSimilarity {
			kept = append(kept, hit)
		}
	}
	sort.SliceStable(kept, func(i, j int) bool {
		si, sj := SearchScore(kept[i], now), SearchScore(kept[j], now)
		if si != sj {
			return si > sj
		}
		if !kept[i].Observation.LastMentionedAt.Equal(kept[j].Observation.LastMentionedAt) {
			return kept[i].Observation.LastMentionedAt.After(kept[j].Observation.LastMentionedAt)
		}
		return kept[i].Observation.ID.String() < kept[j].Observation.ID.String()
	})
	if limit > 0 && len(kept) > limit {
		kept = kept[:limit]
	}
	return kept
}

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
// worker-side vector lookup is SimilarObservations, which stays raw-cosine).
// Tags (lowercase slugs) are matched case-insensitively against observation
// entities. Candidates come back cosine-ordered from Postgres, then hits below
// SearchMinSimilarity are dropped and the rest re-ranked with the strength
// blend (SearchScore) before the limit applies. Before the observations
// migration lands the table reads as empty.
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
		vector, err = s.EmbedQuery(ctx, req.OrgID, req.Query)
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
	candidateLimit := limit * searchCandidateFactor
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
	args = append(args, literal, candidateLimit)

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
	return filterAndRankSearchHits(hits, time.Now().UTC(), limit), nil
}

// isUndefinedTable reports whether err is Postgres undefined_table (42P01):
// reads treat not-yet-migrated memory tables as empty so rollout ordering
// (deploy before/with the migration) never breaks recall.
func isUndefinedTable(err error) bool {
	return err != nil && strings.Contains(err.Error(), "SQLSTATE 42P01")
}
