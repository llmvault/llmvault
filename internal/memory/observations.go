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

	"github.com/usehivy/hivy/internal/goroutine"
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
// content — the same content normalization the reflection store uses. Agent
// ownership lives in a dedicated column, so the fingerprint is content-only.
func SuppressionFingerprint(content string) string {
	normalized := strings.ToLower(strings.Join(strings.Fields(content), " "))
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}

// CreateObservationRequest inserts one consolidated observation.
type CreateObservationRequest struct {
	OrgID           uuid.UUID
	AgentID         uuid.UUID
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
	if req.AgentID == uuid.Nil {
		return nil, fmt.Errorf("agent_id is required")
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
		AgentID:           req.AgentID,
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
// in the owning agent's suppression list, and refreshes that agent's digest.
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
			Omit("Org", "Agent").
			Create(&model.MemorySuppression{
				OrgID:              obs.OrgID,
				AgentID:            obs.AgentID,
				ContentFingerprint: SuppressionFingerprint(obs.Content),
			}).Error
	})
	if err != nil {
		return fmt.Errorf("forget observation: %w", err)
	}
	s.recomputeDigestsInBackground(ctx, obs.OrgID, obs.AgentID)
	return nil
}

// recomputeDigestsInBackground refreshes the owning agent's memory digest
// after an observation mutation. Best-effort failures are repaired by the
// periodic consolidation sweep.
func (s *Service) recomputeDigestsInBackground(ctx context.Context, orgID, agentID uuid.UUID) {
	bg := context.WithoutCancel(ctx)
	goroutine.Go(bg, func(bg context.Context) {
		if err := s.RecomputeAgentDigest(bg, orgID, agentID); err != nil {
			logging.FromContext(bg).WarnContext(bg, "recompute agent memory digest",
				"error", err, "agent_id", agentID)
		}
	})
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
