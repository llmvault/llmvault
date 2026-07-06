package tasks

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/memory"
	"github.com/usehivy/hivy/internal/model"
)

func (h *SessionReflectionHandler) storeMemories(
	ctx context.Context,
	session model.Session,
	events []model.SessionEvent,
	identities map[uuid.UUID]reflectionIdentity,
	candidates []reflectionMemoryCandidate,
) error {
	if len(candidates) == 0 {
		return nil
	}
	service := memory.NewService(memory.Config{
		DB:           h.db,
		CacheManager: h.cacheManager,
		EnqueueEmbed: func(ctx context.Context, id uuid.UUID, revision int) error {
			return EnqueueMemoryEmbed(ctx, h.enqueuer, id, revision)
		},
		EmbeddingModel: h.memoryCfg.Model,
		EmbeddingDim:   h.memoryCfg.Dim,
	})
	byID := make(map[uuid.UUID]model.SessionEvent, len(events))
	for _, event := range events {
		byID[event.ID] = event
	}
	for _, candidate := range candidates {
		req := h.memoryCreateRequest(session, byID, identities, candidate)
		if req.MemoryFingerprint == "" || h.memoryExists(ctx, req) {
			continue
		}
		if _, err := service.Create(ctx, req); err != nil && !isReflectionDuplicate(err) {
			return err
		}
	}
	return nil
}

func (h *SessionReflectionHandler) memoryCreateRequest(
	session model.Session,
	events map[uuid.UUID]model.SessionEvent,
	identities map[uuid.UUID]reflectionIdentity,
	candidate reflectionMemoryCandidate,
) memory.CreateRequest {
	sourceEventID, identity := strongestReflectionSource(events, identities, candidate.SourceEventIDs)
	channelID := session.ChannelID
	tags := append([]string{}, candidate.Tags...)
	tags = append(tags, "reflection", candidate.Kind)
	metadata := model.JSON{
		"source":             "reflection",
		"kind":               candidate.Kind,
		"confidence":         candidate.Confidence,
		"source_event_ids":   candidate.SourceEventIDs,
		"actor_display_name": firstNonEmptyString(candidate.ActorDisplayName, identity.DisplayName),
		"actor_external_ref": firstNonEmptyString(candidate.ActorExternalRef, identity.ExternalRef),
	}
	fingerprint := reflectionMemoryFingerprint(session.OrgID, &channelID, candidate.Content)
	return memory.CreateRequest{
		OrgID:             session.OrgID,
		ChannelID:         &channelID,
		Content:           candidate.Content,
		MemoryFingerprint: fingerprint,
		Tags:              tags,
		Metadata:          metadata,
		SourceSessionID:   &session.ID,
		SourceEventID:     sourceEventID,
	}
}

func strongestReflectionSource(
	events map[uuid.UUID]model.SessionEvent,
	identities map[uuid.UUID]reflectionIdentity,
	rawIDs []string,
) (*uuid.UUID, reflectionIdentity) {
	for _, raw := range rawIDs {
		id, err := uuid.Parse(strings.TrimSpace(raw))
		if err != nil {
			continue
		}
		if _, ok := events[id]; ok {
			sourceID := id
			return &sourceID, identities[id]
		}
	}
	return nil, reflectionIdentity{}
}

func (h *SessionReflectionHandler) memoryExists(ctx context.Context, req memory.CreateRequest) bool {
	q := h.db.WithContext(ctx).Model(&model.AgentMemory{}).
		Where("org_id = ? AND memory_fingerprint = ? AND archived_at IS NULL", req.OrgID, req.MemoryFingerprint)
	if req.ChannelID == nil {
		q = q.Where("channel_id IS NULL")
	} else {
		q = q.Where("channel_id = ?", *req.ChannelID)
	}
	var count int64
	return q.Count(&count).Error == nil && count > 0
}

func reflectionMemoryFingerprint(orgID uuid.UUID, channelID *uuid.UUID, content string) string {
	parts := []string{
		orgID.String(),
		uuidPart(channelID),
		strings.ToLower(strings.Join(strings.Fields(content), " ")),
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])
}

func uuidPart(value *uuid.UUID) string {
	if value == nil {
		return ""
	}
	return value.String()
}

func isReflectionDuplicate(err error) bool {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func eventAfterReflectionCursor(event model.SessionEvent, state model.SessionReflectionState) bool {
	if state.LastReflectedEventAt == nil || state.LastReflectedEventID == nil {
		return true
	}
	if event.EventAt.After(*state.LastReflectedEventAt) {
		return true
	}
	return event.EventAt.Equal(*state.LastReflectedEventAt) && event.ID.String() > state.LastReflectedEventID.String()
}

func (h *SessionReflectionHandler) loadExistingMemories(ctx context.Context, sessionID uuid.UUID) string {
	var rows []model.AgentMemory
	if err := h.db.WithContext(ctx).
		Where("source_session_id = ? AND archived_at IS NULL", sessionID).
		Order("created_at DESC").
		Limit(50).
		Find(&rows).Error; err != nil {
		return ""
	}
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		lines = append(lines, "- "+row.Content)
	}
	return strings.Join(lines, "\n")
}

func (h *SessionReflectionHandler) loadReflectionUserNames(ctx context.Context, session model.Session, events []model.SessionEvent) map[uuid.UUID]string {
	ids := map[uuid.UUID]bool{}
	if session.CreatedBy != nil {
		ids[*session.CreatedBy] = true
	}
	for _, event := range events {
		if event.ActorUserID != nil {
			ids[*event.ActorUserID] = true
		}
	}
	values := make([]uuid.UUID, 0, len(ids))
	for id := range ids {
		values = append(values, id)
	}
	if len(values) == 0 {
		return nil
	}
	var users []model.User
	if err := h.db.WithContext(ctx).Select("id", "name").Where("id IN ?", values).Find(&users).Error; err != nil {
		return nil
	}
	out := make(map[uuid.UUID]string, len(users))
	for _, user := range users {
		out[user.ID] = strings.TrimSpace(user.Name)
	}
	return out
}
