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

	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/model"
)

// planReminderSource labels transcript events and queue rows produced by the
// plan-turn reminder, mirroring scheduleSource / triggerConversationSource. The
// frontend attributes source "system" to "Automated · System".
const planReminderSource = "system"

// planReminderEventIDPrefix identifies a reminder in the transcript so redeliveries
// dedupe (stable id per completed turn) and so the loop guard can recognise its
// own last message and refuse to nag twice in a row.
const planReminderEventIDPrefix = "plan-reminder:"

func init() {
	RegisterTaskBuilder(TypePlanTurnReminder, func(payload []byte) (*asynq.Task, []asynq.Option, error) {
		var p PlanTurnReminderPayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return nil, nil, fmt.Errorf("unmarshal plan turn reminder payload: %w", err)
		}
		return NewPlanTurnReminderTask(p)
	})
}

// PlanTurnReminderPayload carries the session and the completed turn that
// triggered the check. TurnID scopes the reminder's stable event id.
type PlanTurnReminderPayload struct {
	SessionID uuid.UUID `json:"session_id"`
	TurnID    string    `json:"turn_id"`
}

func NewPlanTurnReminderTask(payload PlanTurnReminderPayload) (*asynq.Task, []asynq.Option, error) {
	if payload.SessionID == uuid.Nil {
		return nil, nil, fmt.Errorf("session_id is required")
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal plan turn reminder payload: %w", err)
	}
	opts := []asynq.Option{
		asynq.Queue(QueueDefault),
		asynq.MaxRetry(3),
		asynq.Timeout(time.Minute),
	}
	return asynq.NewTask(TypePlanTurnReminder, encoded), opts, nil
}

// EnqueuePlanTurnReminder schedules the after-turn plan check. It is enqueued
// from the runtime-stream projector's turn-completed path so the (potentially
// DB-heavy) plan read and message injection stay off the hot streaming path.
func EnqueuePlanTurnReminder(ctx context.Context, enq enqueue.TaskEnqueuer, sessionID uuid.UUID, turnID string) error {
	if enq == nil {
		return nil
	}
	task, opts, err := NewPlanTurnReminderTask(PlanTurnReminderPayload{SessionID: sessionID, TurnID: turnID})
	if err != nil {
		return err
	}
	if _, err := enq.EnqueueContext(ctx, task, opts...); err != nil {
		return err
	}
	return nil
}

// PlanTurnReminderHandler injects a system reminder when an agent ends a turn
// with a plan that still has non-terminal items. It reuses the automated-message
// machinery (ensureAutomatedSessionEvent + a linked queue row + the generic
// session delivery drain) rather than posting to the runtime directly.
type PlanTurnReminderHandler struct {
	db       *gorm.DB
	enqueuer enqueue.TaskEnqueuer
}

func NewPlanTurnReminderHandler(db *gorm.DB, enqueuer enqueue.TaskEnqueuer) *PlanTurnReminderHandler {
	return &PlanTurnReminderHandler{db: db, enqueuer: enqueuer}
}

func (h *PlanTurnReminderHandler) Handle(ctx context.Context, task *asynq.Task) error {
	if h == nil || h.db == nil {
		return nil
	}
	var payload PlanTurnReminderPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal plan turn reminder payload: %w", err)
	}
	return h.remind(ctx, payload)
}

func (h *PlanTurnReminderHandler) remind(ctx context.Context, payload PlanTurnReminderPayload) error {
	var session model.Session
	if err := h.db.WithContext(ctx).First(&session, "id = ?", payload.SessionID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return fmt.Errorf("load session for plan reminder: %w", err)
	}
	// A new turn is already running (a queued or freshly delivered message took
	// over); the agent is busy, so do not interrupt it.
	if session.AgentTurnStatus != model.SessionAgentTurnIdle {
		return nil
	}
	// The agent is about to run again anyway — a message is still queued.
	pending, err := h.countPending(ctx, session.ID)
	if err != nil {
		return err
	}
	if pending > 0 {
		return nil
	}
	incomplete, err := h.incompletePlanItems(ctx, session.ID)
	if err != nil {
		return err
	}
	if len(incomplete) == 0 {
		return nil
	}
	created, err := h.enqueueReminder(ctx, session, payload.TurnID, incomplete)
	if err != nil {
		return err
	}
	if !created {
		return nil
	}
	if err := EnqueueSessionMessageDeliver(ctx, h.enqueuer, session.ID); err != nil {
		return fmt.Errorf("enqueue plan reminder delivery: %w", err)
	}
	return nil
}

func (h *PlanTurnReminderHandler) countPending(ctx context.Context, sessionID uuid.UUID) (int64, error) {
	var pending int64
	if err := h.db.WithContext(ctx).Model(&model.SessionMessageQueue{}).
		Where("session_id = ? AND status <> ?", sessionID, "delivered").
		Count(&pending).Error; err != nil {
		return 0, fmt.Errorf("count pending session messages: %w", err)
	}
	return pending, nil
}

// enqueueReminder persists the reminder transcript event and its queue row under
// a session lock. It returns created=false — without inserting anything — when
// the loop guard fires (the most recent queued message was itself a reminder) or
// when this turn's reminder already exists (redelivery). Only a created=true row
// warrants firing the delivery drain.
func (h *PlanTurnReminderHandler) enqueueReminder(ctx context.Context, session model.Session, turnID string, incomplete []planItem) (bool, error) {
	eventID := planReminderEventIDPrefix + session.ID.String() + ":" + defaultTurnID(turnID)
	text := planReminderText(incomplete)
	created := false
	err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		created = false
		var locked model.Session
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&locked, "id = ?", session.ID).Error; err != nil {
			return fmt.Errorf("lock session for plan reminder: %w", err)
		}
		// Loop guard: if the newest queued message is already a plan reminder, do
		// not nag again. A fresh reminder is only allowed once some other message
		// has been delivered in between.
		wasReminder, err := latestQueuedMessageIsReminder(tx, locked.ID)
		if err != nil {
			return err
		}
		if wasReminder {
			return nil
		}
		payload := model.JSON{
			"text":            text,
			"source":          planReminderSource,
			"plan_incomplete": len(incomplete),
		}
		event, err := ensureAutomatedSessionEvent(tx, locked, eventID, planReminderSource, payload)
		if err != nil {
			return err
		}
		var existing model.SessionMessageQueue
		err = tx.Where("session_id = ? AND session_event_id = ?", locked.ID, event.ID).First(&existing).Error
		if err == nil {
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("load plan reminder queue row: %w", err)
		}
		seq, err := nextSessionQueueSequence(tx, locked.ID)
		if err != nil {
			return err
		}
		sessionEventID := event.ID
		queue := model.SessionMessageQueue{
			OrgID:           locked.OrgID,
			SessionID:       locked.ID,
			SessionEventID:  &sessionEventID,
			MessageText:     text,
			MessagePayload:  payload,
			SequenceNumber:  seq,
			Status:          "pending",
			Model:           locked.Model,
			ReasoningEffort: locked.ReasoningEffort,
		}
		if err := tx.Create(&queue).Error; err != nil {
			return fmt.Errorf("create plan reminder queue row: %w", err)
		}
		created = true
		return nil
	})
	if err != nil {
		return false, err
	}
	return created, nil
}

// latestQueuedMessageIsReminder reports whether the highest-sequence queue row
// for the session was produced by the plan reminder (its linked transcript event
// id carries the reminder prefix). This is the loop-protection signal: it is true
// exactly when the last thing delivered to the session was a reminder.
func latestQueuedMessageIsReminder(tx *gorm.DB, sessionID uuid.UUID) (bool, error) {
	var latest model.SessionMessageQueue
	err := tx.Where("session_id = ?", sessionID).
		Order("sequence_number DESC").
		First(&latest).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("load latest queued message: %w", err)
	}
	if latest.SessionEventID == nil {
		return false, nil
	}
	var event model.SessionEvent
	if err := tx.Select("event_id").First(&event, "id = ?", *latest.SessionEventID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("load latest queued message event: %w", err)
	}
	return strings.HasPrefix(event.EventID, planReminderEventIDPrefix), nil
}

func defaultTurnID(turnID string) string {
	if trimmed := strings.TrimSpace(turnID); trimmed != "" {
		return trimmed
	}
	return "unknown"
}
