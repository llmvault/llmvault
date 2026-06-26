package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hibiken/asynq"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/model"
)

type ModelUsageWritePayload struct {
	Generation   model.Generation    `json:"generation"`
	SessionEvent *model.SessionEvent `json:"session_event,omitempty"`
}

func init() {
	RegisterTaskBuilder(TypeModelUsageWrite, func(payload []byte) (*asynq.Task, []asynq.Option, error) {
		var p ModelUsageWritePayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return nil, nil, fmt.Errorf("unmarshal model usage payload: %w", err)
		}
		return NewModelUsageWriteTask(p)
	})
}

func NewModelUsageWriteTask(payload ModelUsageWritePayload) (*asynq.Task, []asynq.Option, error) {
	if strings.TrimSpace(payload.Generation.ID) == "" {
		return nil, nil, fmt.Errorf("generation id is required")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal model usage payload: %w", err)
	}
	opts := []asynq.Option{
		asynq.Queue(QueueBulk),
		asynq.MaxRetry(5),
		asynq.Timeout(10 * time.Second),
		asynq.TaskID("model_usage:" + payload.Generation.ID),
	}
	return asynq.NewTask(TypeModelUsageWrite, body), opts, nil
}

func EnqueueModelUsageWrite(ctx context.Context, enq enqueue.TaskEnqueuer, payload ModelUsageWritePayload) error {
	if enq == nil {
		return nil
	}
	task, opts, err := NewModelUsageWriteTask(payload)
	if err != nil {
		return err
	}
	if _, err := enq.EnqueueContext(ctx, task, opts...); err != nil {
		if errors.Is(err, asynq.ErrDuplicateTask) || errors.Is(err, asynq.ErrTaskIDConflict) {
			return nil
		}
		return fmt.Errorf("enqueue model usage write: %w", err)
	}
	return nil
}

type ModelUsageHandler struct {
	db *gorm.DB
}

func NewModelUsageHandler(db *gorm.DB) *ModelUsageHandler {
	return &ModelUsageHandler{db: db}
}

func (h *ModelUsageHandler) Handle(ctx context.Context, task *asynq.Task) error {
	var payload ModelUsageWritePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal model usage payload: %w", err)
	}
	return WriteModelUsage(ctx, h.db, payload)
}

func WriteModelUsage(ctx context.Context, db *gorm.DB, payload ModelUsageWritePayload) error {
	if db == nil {
		return fmt.Errorf("database is required")
	}
	if strings.TrimSpace(payload.Generation.ID) == "" {
		return fmt.Errorf("generation id is required")
	}
	now := time.Now().UTC()
	if payload.Generation.CreatedAt.IsZero() {
		payload.Generation.CreatedAt = now
	}
	if payload.SessionEvent != nil {
		if payload.SessionEvent.EventAt.IsZero() {
			payload.SessionEvent.EventAt = payload.Generation.CreatedAt
		}
		if payload.SessionEvent.CreatedAt.IsZero() {
			payload.SessionEvent.CreatedAt = now
		}
	}

	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Omit(clause.Associations).Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "id"}},
			DoNothing: true,
		}).Create(&payload.Generation).Error; err != nil {
			return fmt.Errorf("create generation: %w", err)
		}
		if payload.SessionEvent == nil {
			return nil
		}
		if err := tx.Omit(clause.Associations).Clauses(clause.OnConflict{
			Columns:     []clause.Column{{Name: "session_id"}, {Name: "event_id"}},
			TargetWhere: clause.Where{Exprs: []clause.Expression{gorm.Expr("event_id IS NOT NULL")}},
			DoNothing:   true,
		}).Create(payload.SessionEvent).Error; err != nil {
			return fmt.Errorf("create session event: %w", err)
		}
		return nil
	})
}
