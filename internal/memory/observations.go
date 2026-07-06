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
	goroutine.Go(bg, func(bg context.Context) {
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
