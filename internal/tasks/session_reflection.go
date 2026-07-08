package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/cache"
	"github.com/usehivy/hivy/internal/credentials"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/memory"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/registry"
	"github.com/usehivy/hivy/internal/trigger/hivy"
)

const (
	sessionReflectionLockTTL      = 5 * time.Minute
	sessionReflectionRetryBackoff = time.Hour
	sessionReflectionEventLimit   = 120
)

type SessionReflectionHandler struct {
	db                  *gorm.DB
	cacheManager        *cache.Manager
	enqueuer            enqueue.TaskEnqueuer
	memoryCfg           MemoryEmbeddingConfig
	reg                 *registry.Registry
	loadCredential      reflectionCredentialLoader
	newCompletionClient sessionNameClientFactory
	now                 func() time.Time
}

type sessionReflectionClaim struct {
	Session        model.Session
	State          model.SessionReflectionState
	ThroughEventID uuid.UUID
	ThroughEventAt time.Time
	Skip           bool
}

func NewSessionReflectionHandler(db *gorm.DB, cacheManager *cache.Manager, enqueuer enqueue.TaskEnqueuer, memoryCfg MemoryEmbeddingConfig) *SessionReflectionHandler {
	return &SessionReflectionHandler{
		db:                  db,
		cacheManager:        cacheManager,
		enqueuer:            enqueuer,
		memoryCfg:           memoryCfg,
		reg:                 registry.Global(),
		loadCredential:      loadReflectionCredential,
		newCompletionClient: hivy.NewCompletionClient,
	}
}

func (h *SessionReflectionHandler) Handle(ctx context.Context, task *asynq.Task) error {
	var payload SessionReflectionPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal session reflection payload: %w", err)
	}
	now := time.Now().UTC()
	if h.now != nil {
		now = h.now().UTC()
	}
	claim, err := h.claim(ctx, payload.SessionID, now)
	if err != nil || claim.Skip {
		return err
	}
	events, err := h.loadEvents(ctx, claim)
	if err != nil {
		_ = h.markFailed(ctx, payload.SessionID, err, nil)
		return err
	}
	if len(events) == 0 {
		return h.release(ctx, payload.SessionID)
	}
	userNames := h.loadReflectionUserNames(ctx, claim.Session, events)
	channelName := h.loadReflectionChannelName(ctx, claim.Session)
	transcript, identities := renderSessionReflectionTranscript(claim.Session, channelName, events, userNames)
	existing := h.loadExistingMemories(ctx, claim.Session.ID)
	cred, err := h.reflectionCredential(ctx)
	if err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("session reflection credential unavailable: %w", err), map[string]any{
			"session_id": payload.SessionID.String(),
		})
		lockUntil := now.Add(sessionReflectionRetryBackoff)
		_ = h.markFailed(ctx, payload.SessionID, err, &lockUntil)
		return reflectionCredentialError(err)
	}
	client := h.completionClient(cred)
	channelMission := h.loadChannelMission(ctx, claim.Session)
	result, _, err := generateSessionReflection(ctx, client, cred.modelID, cred.temperature, transcript, existing, channelMission)
	if err != nil {
		_ = h.markFailed(ctx, payload.SessionID, err, nil)
		return err
	}
	stored, err := h.storeMemories(ctx, claim.Session, events, identities, result.Memories)
	if err != nil {
		_ = h.markFailed(ctx, payload.SessionID, err, nil)
		return err
	}
	if stored > 0 {
		h.enqueueConsolidation(ctx, claim.Session)
	}
	return h.complete(ctx, payload.SessionID, events[len(events)-1])
}

func (h *SessionReflectionHandler) loadEvents(ctx context.Context, claim sessionReflectionClaim) ([]model.SessionEvent, error) {
	q := h.db.WithContext(ctx).
		Where("session_id = ? AND (durability = ? OR durability = '')", claim.Session.ID, "durable").
		Where("(event_at < ? OR (event_at = ? AND id <= ?))", claim.ThroughEventAt, claim.ThroughEventAt, claim.ThroughEventID)
	if claim.State.LastReflectedEventAt != nil && claim.State.LastReflectedEventID != nil {
		q = q.Where("(event_at > ? OR (event_at = ? AND id > ?))",
			*claim.State.LastReflectedEventAt, *claim.State.LastReflectedEventAt, *claim.State.LastReflectedEventID)
	}
	var events []model.SessionEvent
	err := q.Order("event_at ASC, id ASC").Limit(sessionReflectionEventLimit).Find(&events).Error
	if err != nil {
		return nil, fmt.Errorf("load reflection events: %w", err)
	}
	return events, nil
}

func (h *SessionReflectionHandler) loadReflectionChannelName(ctx context.Context, session model.Session) string {
	if session.ChannelID == uuid.Nil {
		return ""
	}
	var channel model.Channel
	if err := h.db.WithContext(ctx).Select("id", "name").
		First(&channel, "id = ?", session.ChannelID).Error; err != nil {
		return ""
	}
	return strings.TrimSpace(channel.Name)
}

// loadChannelMission fetches the channel's memory mission for the extraction
// prompt. Errors degrade to the base guidelines ("" mission) — the reflection
// run must not fail because the mission lookup did.
func (h *SessionReflectionHandler) loadChannelMission(ctx context.Context, session model.Session) string {
	if session.ChannelID == uuid.Nil {
		return ""
	}
	mission, err := memory.ChannelMission(ctx, h.db, session.OrgID, session.ChannelID)
	if err != nil {
		logging.FromContext(ctx).WarnContext(ctx, "load channel memory mission failed; using base guidelines",
			"channel_id", session.ChannelID.String(), "error", err)
		return ""
	}
	return mission
}

// enqueueConsolidation chains a consolidation run after new memories were
// stored. Log-and-continue: reflection success never depends on the enqueue.
func (h *SessionReflectionHandler) enqueueConsolidation(ctx context.Context, session model.Session) {
	if h.enqueuer == nil || session.ChannelID == uuid.Nil {
		return
	}
	if err := EnqueueMemoryConsolidate(ctx, h.enqueuer, session.OrgID, session.ChannelID); err != nil {
		logging.FromContext(ctx).WarnContext(ctx, "enqueue memory consolidation after reflection failed",
			"org_id", session.OrgID.String(), "channel_id", session.ChannelID.String(), "error", err)
	}
}

func (h *SessionReflectionHandler) reflectionCredential(ctx context.Context) (*reflectionCredential, error) {
	loader := h.loadCredential
	if loader == nil {
		loader = loadReflectionCredential
	}
	return loader(ctx, h.db, h.cacheManager, h.registry())
}

func (h *SessionReflectionHandler) completionClient(cred *reflectionCredential) hivy.CompletionClient {
	factory := h.newCompletionClient
	if factory == nil {
		factory = hivy.NewCompletionClient
	}
	return factory(cred.credential, cred.apiKey)
}

func (h *SessionReflectionHandler) registry() *registry.Registry {
	if h != nil && h.reg != nil {
		return h.reg
	}
	return registry.Global()
}

// reflectionCredentialError decides whether a credential failure should also
// fail the asynq task. A missing system credential is already logged and
// recorded on session_reflection_states (status=failed, last_error, retry
// backoff); retrying the task immediately cannot fix it, so asynq sees nil.
func reflectionCredentialError(err error) error {
	if errors.Is(err, credentials.ErrNoSystemCredential) {
		return nil
	}
	return err
}

func (h *SessionReflectionHandler) release(ctx context.Context, sessionID uuid.UUID) error {
	return h.db.WithContext(ctx).Model(&model.SessionReflectionState{}).
		Where("session_id = ?", sessionID).
		Updates(map[string]any{
			"status":       model.SessionReflectionStatusIdle,
			"locked_until": nil,
			"updated_at":   time.Now().UTC(),
		}).Error
}

func (h *SessionReflectionHandler) complete(ctx context.Context, sessionID uuid.UUID, event model.SessionEvent) error {
	updates := map[string]any{
		"last_reflected_event_id": event.ID,
		"last_reflected_event_at": event.EventAt,
		"status":                  model.SessionReflectionStatusIdle,
		"locked_until":            nil,
		"last_error":              "",
		"updated_at":              time.Now().UTC(),
	}
	if event.RuntimeSeq != nil {
		updates["last_reflected_runtime_seq"] = *event.RuntimeSeq
	}
	return h.db.WithContext(ctx).Model(&model.SessionReflectionState{}).
		Where("session_id = ?", sessionID).
		Updates(updates).Error
}

func (h *SessionReflectionHandler) markFailed(ctx context.Context, sessionID uuid.UUID, cause error, lockedUntil *time.Time) error {
	message := strings.TrimSpace(cause.Error())
	if len(message) > 500 {
		message = message[:500]
	}
	if lockedUntil == nil {
		nextScan := time.Now().UTC().Add(time.Minute)
		lockedUntil = &nextScan
	}
	return h.db.WithContext(ctx).Model(&model.SessionReflectionState{}).
		Where("session_id = ?", sessionID).
		Updates(map[string]any{
			"status":       model.SessionReflectionStatusFailed,
			"locked_until": lockedUntil,
			"last_error":   message,
			"updated_at":   time.Now().UTC(),
		}).Error
}
