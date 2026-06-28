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
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/cache"
	"github.com/usehivy/hivy/internal/credentials"
	"github.com/usehivy/hivy/internal/enqueue"
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
	loadCredential      sessionNameCredentialLoader
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
		loadCredential:      loadSessionNameCredential,
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
	transcript, identities := renderSessionReflectionTranscript(claim.Session, events, userNames)
	existing := h.loadExistingMemories(ctx, claim.Session.ID)
	cred, err := h.reflectionCredential(ctx)
	if err != nil {
		lockUntil := now.Add(sessionReflectionRetryBackoff)
		_ = h.markFailed(ctx, payload.SessionID, err, &lockUntil)
		return reflectionCredentialError(err)
	}
	client := h.completionClient(cred)
	result, err := generateSessionReflection(ctx, client, cred.modelID, transcript, existing)
	if err != nil {
		_ = h.markFailed(ctx, payload.SessionID, err, nil)
		return err
	}
	if err := h.storeMemories(ctx, claim.Session, events, identities, result.Memories); err != nil {
		_ = h.markFailed(ctx, payload.SessionID, err, nil)
		return err
	}
	return h.complete(ctx, payload.SessionID, events[len(events)-1])
}

func (h *SessionReflectionHandler) claim(ctx context.Context, sessionID uuid.UUID, now time.Time) (sessionReflectionClaim, error) {
	if h == nil || h.db == nil || sessionID == uuid.Nil {
		return sessionReflectionClaim{Skip: true}, nil
	}
	var out sessionReflectionClaim
	err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var session model.Session
		if err := tx.First(&session, "id = ?", sessionID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				out.Skip = true
				return nil
			}
			return fmt.Errorf("load reflection session: %w", err)
		}
		latest, ok, err := loadLatestReflectableEvent(tx, sessionID)
		if err != nil || !ok {
			out.Skip = !ok
			return err
		}
		if session.Status != "active" || session.AgentTurnStatus != model.SessionAgentTurnIdle ||
			latest.EventAt.After(now.Add(-sessionReflectionIdleDelay)) {
			out.Skip = true
			return nil
		}
		state, err := claimReflectionState(tx, session, now)
		if err != nil {
			return err
		}
		if state.LockedUntil != nil && state.LockedUntil.After(now) && state.Status == model.SessionReflectionStatusRunning {
			out.Skip = true
			return nil
		}
		if !eventAfterReflectionCursor(latest, state) {
			out.Skip = true
			return nil
		}
		updates := map[string]any{
			"org_id":       session.OrgID,
			"agent_id":     session.AgentID,
			"status":       model.SessionReflectionStatusRunning,
			"locked_until": now.Add(sessionReflectionLockTTL),
			"last_error":   "",
			"updated_at":   now,
		}
		if err := tx.Model(&model.SessionReflectionState{}).
			Where("session_id = ?", session.ID).
			Updates(updates).Error; err != nil {
			return fmt.Errorf("lock reflection state: %w", err)
		}
		out.Session = session
		out.State = state
		out.ThroughEventID = latest.ID
		out.ThroughEventAt = latest.EventAt
		return nil
	})
	return out, err
}

func claimReflectionState(tx *gorm.DB, session model.Session, now time.Time) (model.SessionReflectionState, error) {
	state := model.SessionReflectionState{
		SessionID: session.ID,
		OrgID:     session.OrgID,
		AgentID:   session.AgentID,
		Status:    model.SessionReflectionStatusIdle,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&state).Error; err != nil {
		return model.SessionReflectionState{}, fmt.Errorf("ensure reflection state: %w", err)
	}
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&state, "session_id = ?", session.ID).Error; err != nil {
		return model.SessionReflectionState{}, fmt.Errorf("load reflection state: %w", err)
	}
	return state, nil
}

func loadLatestReflectableEvent(db *gorm.DB, sessionID uuid.UUID) (model.SessionEvent, bool, error) {
	var event model.SessionEvent
	err := db.Where("session_id = ? AND (durability = ? OR durability = '')", sessionID, "durable").
		Order("event_at DESC, id DESC").
		First(&event).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.SessionEvent{}, false, nil
	}
	if err != nil {
		return model.SessionEvent{}, false, fmt.Errorf("load latest reflection event: %w", err)
	}
	return event, true, nil
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

func (h *SessionReflectionHandler) reflectionCredential(ctx context.Context) (*sessionNameCredential, error) {
	loader := h.loadCredential
	if loader == nil {
		loader = loadSessionNameCredential
	}
	return loader(ctx, h.db, h.cacheManager, h.registry())
}

func (h *SessionReflectionHandler) completionClient(cred *sessionNameCredential) hivy.CompletionClient {
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

func reflectionCredentialError(err error) error {
	if errors.Is(err, credentials.ErrNoSystemCredential) || errors.Is(err, errSessionNameModelUnavailable) {
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
