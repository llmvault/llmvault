package handler

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/tasks"
)

type sessionMessageDeliveryIntent struct {
	Session       model.Session
	Command       tasks.SessionMessageCommand
	Queued        bool
	DispatchQueue bool
}

type sessionMessageDeliveryOptions struct {
	Model            string
	ReasoningEffort  string
	ClearLastOutcome bool
}

func (h *SessionHandler) runtimeDeliveryConfigured() bool {
	return h != nil && h.orchestrator != nil && h.compileDeps.EncKey != nil
}

func (h *SessionHandler) createInitialSessionMessageIntent(ctx context.Context, session *model.Session, userID *uuid.UUID, text string, payload model.JSON) (sessionMessageDeliveryIntent, error) {
	if session == nil || session.ID == uuid.Nil {
		return sessionMessageDeliveryIntent{}, fmt.Errorf("session is required")
	}
	direct := h.runtimeDeliveryConfigured()
	if direct {
		markSessionTurnStarting(session, time.Now())
	}
	err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(session).Error; err != nil {
			return err
		}
		if userID != nil {
			now := time.Now()
			if err := tx.Create(&model.SessionParticipant{
				SessionID: session.ID,
				UserID:    *userID,
				Role:      "owner",
				JoinedAt:  &now,
			}).Error; err != nil {
				return err
			}
		}
		if !direct {
			return h.enqueueSessionMessageCommand(tx, session, userID, text, payload, sessionMessageDeliveryOptions{})
		}
		return nil
	})
	if err != nil {
		return sessionMessageDeliveryIntent{}, err
	}
	return sessionMessageDeliveryIntent{
		Session:       *session,
		Command:       tasks.SessionMessageCommand{ActorUserID: userID, Text: text, Payload: sessionMessageCommandPayload(text, payload)},
		Queued:        !direct,
		DispatchQueue: !direct,
	}, nil
}

func (h *SessionHandler) createSessionOnly(ctx context.Context, session *model.Session, userID *uuid.UUID) error {
	if session == nil || session.ID == uuid.Nil {
		return fmt.Errorf("session is required")
	}
	return h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(session).Error; err != nil {
			return err
		}
		if userID == nil {
			return nil
		}
		now := time.Now()
		return tx.Create(&model.SessionParticipant{
			SessionID: session.ID,
			UserID:    *userID,
			Role:      "owner",
			JoinedAt:  &now,
		}).Error
	})
}

func (h *SessionHandler) createSessionMessageIntent(ctx context.Context, base model.Session, actor *uuid.UUID, text string, payload model.JSON, opts sessionMessageDeliveryOptions) (sessionMessageDeliveryIntent, error) {
	var intent sessionMessageDeliveryIntent
	err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var session model.Session
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", base.ID).
			First(&session).Error; err != nil {
			return fmt.Errorf("lock session for message delivery: %w", err)
		}
		now := time.Now()
		updates := map[string]any{"updated_at": now}
		session.UpdatedAt = now
		if strings.TrimSpace(opts.Model) != "" {
			updates["model"] = strings.TrimSpace(opts.Model)
			updates["reasoning_effort"] = strings.TrimSpace(opts.ReasoningEffort)
			session.Model = strings.TrimSpace(opts.Model)
			session.ReasoningEffort = strings.TrimSpace(opts.ReasoningEffort)
		}
		if opts.ClearLastOutcome {
			updates["agent_turn_last_outcome"] = ""
			session.AgentTurnLastOutcome = ""
		}

		queued := session.AgentTurnStatus == model.SessionAgentTurnActive || !h.runtimeDeliveryConfigured()
		if queued {
			if err := h.enqueueSessionMessageCommand(tx, &session, actor, text, payload, opts); err != nil {
				return err
			}
		} else {
			startedAt := time.Now()
			updates["agent_turn_status"] = model.SessionAgentTurnActive
			updates["agent_turn_id"] = ""
			updates["agent_stream_id"] = ""
			updates["agent_turn_started_at"] = startedAt
			updates["agent_turn_last_outcome"] = ""
			markSessionTurnStarting(&session, startedAt)
		}

		if err := tx.Model(&model.Session{}).Where("id = ?", session.ID).Updates(updates).Error; err != nil {
			return fmt.Errorf("update session message state: %w", err)
		}
		intent = sessionMessageDeliveryIntent{
			Session:       session,
			Command:       tasks.SessionMessageCommand{ActorUserID: actor, Text: text, Payload: sessionMessageCommandPayload(text, payload), Model: opts.Model, ReasoningEffort: opts.ReasoningEffort},
			Queued:        queued,
			DispatchQueue: queued && session.AgentTurnStatus != model.SessionAgentTurnActive,
		}
		return nil
	})
	return intent, err
}

func markSessionTurnStarting(session *model.Session, startedAt time.Time) {
	session.AgentTurnStatus = model.SessionAgentTurnActive
	session.AgentTurnID = ""
	session.AgentStreamID = ""
	session.AgentTurnStartedAt = &startedAt
	session.AgentTurnLastOutcome = ""
}

func (h *SessionHandler) dispatchSessionMessageIntent(ctx context.Context, intent sessionMessageDeliveryIntent) (bool, error) {
	if intent.Queued {
		if intent.DispatchQueue {
			return true, h.enqueueSessionDelivery(ctx, intent.Session.ID)
		}
		return true, nil
	}
	dispatcher := tasks.NewSessionMessageDeliverHandler(h.db, h.orchestrator, h.compileDeps, h.enqueuer)
	delivery, err := dispatcher.DeliverCommand(ctx, intent.Session, intent.Command)
	if err != nil {
		_ = h.releaseDirectSessionTurn(context.WithoutCancel(ctx), intent.Session.ID)
		return false, err
	}
	if err := h.recordDirectSessionDelivery(ctx, intent.Session.ID, delivery); err != nil {
		return false, err
	}
	return false, nil
}

func (h *SessionHandler) recordDirectSessionDelivery(ctx context.Context, sessionID uuid.UUID, delivery *agentruntime.HTTPMessageResponse) error {
	updates := map[string]any{
		"agent_turn_status":       model.SessionAgentTurnActive,
		"agent_turn_last_outcome": "",
	}
	if delivery != nil {
		updates["agent_turn_id"] = strings.TrimSpace(delivery.TurnID)
		updates["agent_stream_id"] = strings.TrimSpace(delivery.StreamID)
	}
	return h.db.WithContext(ctx).Model(&model.Session{}).
		Where("id = ?", sessionID).
		Updates(updates).Error
}

func (h *SessionHandler) releaseDirectSessionTurn(ctx context.Context, sessionID uuid.UUID) error {
	return h.db.WithContext(ctx).Model(&model.Session{}).
		Where("id = ?", sessionID).
		Updates(map[string]any{
			"agent_turn_status":       model.SessionAgentTurnIdle,
			"agent_turn_id":           "",
			"agent_stream_id":         "",
			"agent_turn_started_at":   nil,
			"agent_turn_last_outcome": model.SessionAgentTurnOutcomeFailed,
		}).Error
}
