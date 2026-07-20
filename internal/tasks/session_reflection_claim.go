package tasks

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/runtimeevents"
)

const sessionReflectionMinimumMessages = 5

var sessionReflectionMessageEventTypes = []string{
	runtimeevents.EventUserMessageReceived,
	runtimeevents.EventMessageReceived,
	runtimeevents.EventFinal,
	runtimeevents.EventResponseCompleted,
	runtimeevents.EventQuestionRequested,
	runtimeevents.EventQuestionAnswered,
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
		hasEnoughMessages, err := sessionHasMinimumReflectionMessages(tx, sessionID)
		if err != nil {
			return err
		}
		if !hasEnoughMessages {
			out.Skip = true
			return nil
		}
		latest, ok, err := loadLatestReflectableEvent(tx, sessionID)
		if err != nil || !ok {
			out.Skip = !ok
			return err
		}
		if skipReflectionForSessionState(session, latest, now) {
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

func sessionHasMinimumReflectionMessages(db *gorm.DB, sessionID uuid.UUID) (bool, error) {
	var count int64
	err := db.Model(&model.SessionEvent{}).
		Where("session_id = ?", sessionID).
		Where("event_type IN ?", sessionReflectionMessageEventTypes).
		Where("durability = ? OR durability = ''", "durable").
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("count reflection messages: %w", err)
	}
	return count >= sessionReflectionMinimumMessages, nil
}

// skipReflectionForSessionState gates the near-real-time idle loop: active
// sessions must be turn-idle and quiet for the idle delay. Archived/ended
// sessions run a final pass over their unreflected tail immediately — their
// events are settled by definition.
func skipReflectionForSessionState(session model.Session, latest model.SessionEvent, now time.Time) bool {
	switch session.Status {
	case "active":
		return session.AgentTurnStatus != model.SessionAgentTurnIdle ||
			latest.EventAt.After(now.Add(-sessionReflectionIdleDelay))
	case "archived", "ended":
		return false
	default:
		return true
	}
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
