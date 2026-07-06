package memory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

// ObservationKinds is the closed kind enum for consolidated observations.
// It mirrors the extraction kind enum so facts map 1:1 onto observations.
var ObservationKinds = []string{
	"preference", "rule", "decision", "convention", "org-fact",
	"person", "workaround", "commitment", "finding",
}

// ValidObservationKind reports whether kind is in the observation kind enum.
func ValidObservationKind(kind string) bool {
	for _, known := range ObservationKinds {
		if kind == known {
			return true
		}
	}
	return false
}

// SuppressionFingerprint fingerprints observation content for the
// memory_suppressions table: sha256 of the lowercased, whitespace-collapsed
// content — the same content normalization the reflection store uses. Org and
// channel scope live in dedicated columns, so the fingerprint is content-only.
func SuppressionFingerprint(content string) string {
	normalized := strings.ToLower(strings.Join(strings.Fields(content), " "))
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}

// CreateObservationRequest inserts one consolidated observation.
type CreateObservationRequest struct {
	OrgID           uuid.UUID
	ChannelID       *uuid.UUID // nil = org-wide
	Content         string
	Kind            string
	Entities        []string
	SourceFactIDs   []uuid.UUID
	OccurredStart   *time.Time
	OccurredEnd     *time.Time
	LastMentionedAt time.Time
	ExpiresAt       *time.Time
	HumanVerified   bool
	Metadata        model.JSON
}

// CreateObservation inserts a new observation with a pending embedding.
func (s *Service) CreateObservation(ctx context.Context, req CreateObservationRequest) (*model.AgentObservation, error) {
	if s == nil || s.cfg.DB == nil {
		return nil, fmt.Errorf("memory service is not configured")
	}
	content, err := normalizeContent(req.Content)
	if err != nil {
		return nil, err
	}
	if req.OrgID == uuid.Nil {
		return nil, fmt.Errorf("org_id is required")
	}
	if !ValidObservationKind(req.Kind) {
		return nil, fmt.Errorf("invalid observation kind %q", req.Kind)
	}
	lastMentioned := req.LastMentionedAt
	if lastMentioned.IsZero() {
		lastMentioned = time.Now().UTC()
	}
	proof := len(uniqueUUIDStrings(req.SourceFactIDs))
	if proof < 1 {
		proof = 1
	}
	obs := &model.AgentObservation{
		ID:                uuid.New(),
		OrgID:             req.OrgID,
		ChannelID:         req.ChannelID,
		Content:           content,
		Kind:              req.Kind,
		Entities:          pq.StringArray(NormalizeTags(req.Entities)),
		ProofCount:        proof,
		SourceFactIDs:     pq.StringArray(uniqueUUIDStrings(req.SourceFactIDs)),
		OccurredStart:     req.OccurredStart,
		OccurredEnd:       req.OccurredEnd,
		LastMentionedAt:   lastMentioned,
		ExpiresAt:         req.ExpiresAt,
		HumanVerified:     req.HumanVerified,
		EmbeddingModel:    s.embeddingModel(),
		EmbeddingStatus:   model.AgentMemoryEmbeddingPending,
		EmbeddingRevision: 1,
		Metadata:          normalizeMetadata(req.Metadata),
	}
	if err := s.cfg.DB.WithContext(ctx).Create(obs).Error; err != nil {
		return nil, fmt.Errorf("create observation: %w", err)
	}
	return obs, nil
}

// GetObservation loads one non-archived observation scoped to the org.
func (s *Service) GetObservation(ctx context.Context, orgID, id uuid.UUID) (*model.AgentObservation, error) {
	if s == nil || s.cfg.DB == nil {
		return nil, fmt.Errorf("memory service is not configured")
	}
	var obs model.AgentObservation
	err := s.cfg.DB.WithContext(ctx).
		Where("id = ? AND org_id = ? AND archived_at IS NULL", id, orgID).
		First(&obs).Error
	if err != nil {
		return nil, err
	}
	return &obs, nil
}

// SaveObservationChanges persists mutated fields of an already-loaded
// observation. When contentChanged is true the embedding is invalidated
// (revision bump + pending status) so the caller re-embeds.
func (s *Service) SaveObservationChanges(ctx context.Context, obs *model.AgentObservation, contentChanged bool) error {
	if s == nil || s.cfg.DB == nil || obs == nil {
		return fmt.Errorf("memory service is not configured")
	}
	now := time.Now().UTC()
	updates := map[string]any{
		"content":           obs.Content,
		"kind":              obs.Kind,
		"entities":          obs.Entities,
		"proof_count":       obs.ProofCount,
		"source_fact_ids":   obs.SourceFactIDs,
		"occurred_start":    obs.OccurredStart,
		"occurred_end":      obs.OccurredEnd,
		"last_mentioned_at": obs.LastMentionedAt,
		"expires_at":        obs.ExpiresAt,
		"human_verified":    obs.HumanVerified,
		"metadata":          obs.Metadata,
		"updated_at":        now,
	}
	if contentChanged {
		obs.EmbeddingRevision++
		updates["embedding_revision"] = obs.EmbeddingRevision
		updates["embedding_status"] = model.AgentMemoryEmbeddingPending
		updates["embedding_model"] = s.embeddingModel()
		updates["embedding_error"] = ""
		updates["embedded_at"] = nil
		updates["embedding"] = gorm.Expr("NULL")
		obs.EmbeddingStatus = model.AgentMemoryEmbeddingPending
	}
	if err := s.cfg.DB.WithContext(ctx).Model(&model.AgentObservation{}).
		Where("id = ? AND org_id = ?", obs.ID, obs.OrgID).
		Updates(updates).Error; err != nil {
		return fmt.Errorf("save observation: %w", err)
	}
	return nil
}

// ArchiveObservation soft-deletes an observation, optionally linking the
// observation that supersedes it. Observations are never hard-deleted.
func (s *Service) ArchiveObservation(ctx context.Context, orgID, id uuid.UUID, supersededBy *uuid.UUID) error {
	if s == nil || s.cfg.DB == nil {
		return fmt.Errorf("memory service is not configured")
	}
	now := time.Now().UTC()
	updates := map[string]any{"archived_at": &now, "updated_at": now}
	if supersededBy != nil && *supersededBy != uuid.Nil {
		updates["superseded_by"] = *supersededBy
	}
	res := s.cfg.DB.WithContext(ctx).Model(&model.AgentObservation{}).
		Where("id = ? AND org_id = ? AND archived_at IS NULL", id, orgID).
		Updates(updates)
	if res.Error != nil {
		return fmt.Errorf("archive observation: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// ForgetObservation archives an observation, records its content fingerprint
// in the per-channel suppression list so consolidation cannot resurrect it,
// and refreshes affected channel memory digests in the background — the same
// semantics as the observation delete endpoint.
func (s *Service) ForgetObservation(ctx context.Context, obs *model.AgentObservation) error {
	if s == nil || s.cfg.DB == nil || obs == nil {
		return fmt.Errorf("memory service is not configured")
	}
	now := time.Now().UTC()
	err := s.cfg.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&model.AgentObservation{}).
			Where("id = ? AND org_id = ? AND archived_at IS NULL", obs.ID, obs.OrgID).
			Updates(map[string]any{"archived_at": &now, "updated_at": now})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return tx.Clauses(clause.OnConflict{DoNothing: true}).
			Omit("Org", "Channel").
			Create(&model.MemorySuppression{
				OrgID:              obs.OrgID,
				ChannelID:          obs.ChannelID,
				ContentFingerprint: SuppressionFingerprint(obs.Content),
			}).Error
	})
	if err != nil {
		return fmt.Errorf("forget observation: %w", err)
	}
	s.recomputeDigestsInBackground(ctx, obs.OrgID, obs.ChannelID)
	return nil
}

// recomputeDigestsInBackground refreshes the precomputed channel memory digest
// after an observation mutation so recall reflects it immediately. Best-effort
// in the background: a failed recompute is logged, never surfaced, and the
// periodic consolidation sweep repairs it. For org-wide observations (nil
// channelID) every channel folding org memories in is refreshed.
func (s *Service) recomputeDigestsInBackground(ctx context.Context, orgID uuid.UUID, channelID *uuid.UUID) {
	bg := context.WithoutCancel(ctx)
	go func() {
		if channelID != nil {
			if err := s.RecomputeChannelDigest(bg, orgID, *channelID); err != nil {
				logging.FromContext(bg).WarnContext(bg, "recompute channel memory digest",
					"error", err, "channel_id", *channelID)
			}
			return
		}
		var channelIDs []uuid.UUID
		if err := s.cfg.DB.WithContext(bg).Model(&model.Channel{}).
			Where("org_id = ? AND archived_at IS NULL AND expose_org_memories = true", orgID).
			Pluck("id", &channelIDs).Error; err != nil {
			logging.FromContext(bg).WarnContext(bg, "list channels for digest recompute",
				"error", err, "org_id", orgID)
			return
		}
		for _, id := range channelIDs {
			if err := s.RecomputeChannelDigest(bg, orgID, id); err != nil {
				logging.FromContext(bg).WarnContext(bg, "recompute channel memory digest",
					"error", err, "channel_id", id)
			}
		}
	}()
}

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

// ListUnconsolidatedFacts returns the oldest reflection facts in a channel
// that consolidation has not folded into observations yet.
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
		Where("metadata->>'source' = ?", "reflection").
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

// ChannelsWithUnconsolidatedFacts finds channels holding reflection facts the
// consolidation worker has not processed (the stranded-facts sweep source).
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
	AND metadata->>'source' = 'reflection'
LIMIT ?`, limit).Scan(&rows).Error
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

// LoadObservation loads an observation regardless of archive state (used by
// the observation embed worker's revision check).
func (s *Service) LoadObservation(ctx context.Context, id uuid.UUID) (*model.AgentObservation, error) {
	if s == nil || s.cfg.DB == nil {
		return nil, fmt.Errorf("memory service is not configured")
	}
	var obs model.AgentObservation
	err := s.cfg.DB.WithContext(ctx).Where("id = ?", id).First(&obs).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if err != nil {
		return nil, fmt.Errorf("load observation: %w", err)
	}
	return &obs, nil
}

func uniqueUUIDStrings(ids []uuid.UUID) []string {
	seen := make(map[uuid.UUID]bool, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id == uuid.Nil || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id.String())
	}
	return out
}
