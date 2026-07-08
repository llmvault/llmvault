package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/memory"
	"github.com/usehivy/hivy/internal/model"
)

func (h *MemoryConsolidationHandler) applyCreate(
	ctx context.Context,
	svc *memory.Service,
	payload MemoryConsolidatePayload,
	create resolvedCreate,
	factByID map[uuid.UUID]model.AgentMemory,
	now time.Time,
) (*model.AgentObservation, bool, error) {
	logger := logging.FromContext(ctx)
	humanSource := false
	var occurredStart, occurredEnd *time.Time
	for _, factID := range create.SourceFactIDs {
		fact, ok := factByID[factID]
		if !ok {
			continue
		}
		if factFromHumanActor(fact) {
			humanSource = true
		}
		at := fact.CreatedAt
		if occurredStart == nil || at.Before(*occurredStart) {
			start := at
			occurredStart = &start
		}
		if occurredEnd == nil || at.After(*occurredEnd) {
			end := at
			occurredEnd = &end
		}
	}
	obsChannelID := consolidationCreateChannelID(create.Op.Scope, payload.ChannelID, len(create.SourceFactIDs), humanSource)
	suppressed, err := svc.IsSuppressed(ctx, payload.OrgID, obsChannelID, create.Op.Text)
	if err != nil {
		return nil, false, err
	}
	if suppressed {
		logger.InfoContext(ctx, "consolidation create dropped by suppression list",
			"org_id", payload.OrgID, "channel_id", payload.ChannelID, "reason", create.Op.Reason)
		return nil, false, nil
	}
	kind := create.Op.Kind
	if !memory.ValidObservationKind(kind) {
		kind = "finding"
	}
	metadata := appendObservationAudit(model.JSON{"source": "consolidation"}, "create", create.Op.Reason, create.SourceFactIDs, now, "")
	obs, err := svc.CreateObservation(ctx, memory.CreateObservationRequest{
		OrgID:           payload.OrgID,
		ChannelID:       obsChannelID,
		Content:         create.Op.Text,
		Kind:            kind,
		Entities:        create.Op.Entities,
		SourceFactIDs:   create.SourceFactIDs,
		OccurredStart:   occurredStart,
		OccurredEnd:     occurredEnd,
		LastMentionedAt: now,
		ExpiresAt:       parseConsolidationExpiresAt(create.Op.ExpiresAt),
		Metadata:        metadata,
	})
	if err != nil {
		return nil, false, err
	}
	return obs, true, nil
}

func (h *MemoryConsolidationHandler) applyUpdate(
	ctx context.Context,
	svc *memory.Service,
	orgID uuid.UUID,
	update resolvedUpdate,
	now time.Time,
) (uuid.UUID, bool, error) {
	obs, err := svc.GetObservation(ctx, orgID, update.ObservationID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		logging.FromContext(ctx).WarnContext(ctx, "consolidation update target missing",
			"observation_id", update.ObservationID)
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, err
	}
	contentChanged := applyConsolidationUpdate(obs, update.Op.Text, update.Op.Reason, update.SourceFactIDs, now)
	if err := svc.SaveObservationChanges(ctx, obs, contentChanged); err != nil {
		return uuid.Nil, false, err
	}
	return obs.ID, true, nil
}

func (h *MemoryConsolidationHandler) applyDelete(
	ctx context.Context,
	svc *memory.Service,
	orgID uuid.UUID,
	del resolvedDelete,
	now time.Time,
) error {
	obs, err := svc.GetObservation(ctx, orgID, del.ObservationID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		logging.FromContext(ctx).WarnContext(ctx, "consolidation delete target missing",
			"observation_id", del.ObservationID)
		return nil
	}
	if err != nil {
		return err
	}
	obs.Metadata = appendObservationAudit(obs.Metadata, "delete", del.Op.Reason, nil, now, "")
	if err := svc.SaveObservationChanges(ctx, obs, false); err != nil {
		return err
	}
	if err := svc.ArchiveObservation(ctx, orgID, obs.ID, nil); err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return nil
}

// embedAndDedup embeds each created/updated observation synchronously (the
// dedup probe needs the vector right away; failures fall back to the async
// observation embed queue) and merges near-identical same-scope pairs via the
// tiny merge/keep adjudicator.
func (h *MemoryConsolidationHandler) embedAndDedup(
	ctx context.Context,
	svc *memory.Service,
	orgID uuid.UUID,
	changed []uuid.UUID,
	now time.Time,
) {
	logger := logging.FromContext(ctx)
	for _, obsID := range changed {
		obs, err := svc.GetObservation(ctx, orgID, obsID)
		if err != nil {
			// Archived by an earlier dedup merge in this same pass, or gone.
			continue
		}
		vector, err := h.embedObservation(ctx, svc, obs)
		if err != nil {
			logger.WarnContext(ctx, "observation embed failed; queued for retry",
				"observation_id", obs.ID, "error", err)
			continue
		}
		if err := h.dedupObservation(ctx, svc, obs, vector, now); err != nil {
			logger.WarnContext(ctx, "observation semantic dedup failed",
				"observation_id", obs.ID, "error", err)
		}
	}
}

func (h *MemoryConsolidationHandler) embedObservation(
	ctx context.Context,
	svc *memory.Service,
	obs *model.AgentObservation,
) ([]float32, error) {
	vectors, err := svc.EmbedContents(ctx, obs.OrgID, []string{obs.Content})
	if err != nil {
		if enqueueErr := EnqueueObservationEmbed(ctx, h.enqueuer, obs.ID, obs.EmbeddingRevision); enqueueErr != nil {
			return nil, errors.Join(err, enqueueErr)
		}
		return nil, err
	}
	if _, err := svc.MarkObservationEmbeddingReady(ctx, obs.ID, obs.EmbeddingRevision, vectors[0]); err != nil {
		return nil, err
	}
	return vectors[0], nil
}

func (h *MemoryConsolidationHandler) dedupObservation(
	ctx context.Context,
	svc *memory.Service,
	obs *model.AgentObservation,
	vector []float32,
	now time.Time,
) error {
	obsID := obs.ID
	hits, err := svc.SimilarObservations(ctx, memory.SimilarObservationsRequest{
		OrgID:     obs.OrgID,
		ChannelID: obs.ChannelID, // nil probes org-wide scope only — same scope as obs
		Vector:    vector,
		Limit:     memoryConsolidationTopK,
		ExcludeID: &obsID,
	})
	if err != nil {
		return err
	}
	if len(hits) == 0 || hits[0].Similarity < memoryConsolidationDedupThreshold {
		return nil
	}
	twin := hits[0].Observation
	prompt := fmt.Sprintf(consolidationDedupPrompt, obs.Content, twin.Content)
	raw, err := h.completion()(ctx, "", prompt,
		"memory_dedup", json.RawMessage(consolidationDedupResponseSchema), memoryConsolidationDedupMaxTokens)
	if err != nil {
		return err
	}
	decision, err := parseConsolidationDedupResponse(raw)
	if err != nil {
		return err
	}
	if decision.Action != "merge" {
		return nil
	}
	return h.mergeObservations(ctx, svc, obs, &twin, decision, now)
}

// mergeObservations keeps the older row, merges text, sums proof counts,
// unions source facts, and archives the newer row with superseded_by.
