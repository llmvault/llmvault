package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/model"
)

const githubReviewBatchWindow = 30 * time.Second

type SessionReviewBatchFlushPayload struct {
	QueueID uuid.UUID `json:"queue_id"`
}

func init() {
	RegisterTaskBuilder(TypeSessionReviewBatchFlush, func(payload []byte) (*asynq.Task, []asynq.Option, error) {
		var p SessionReviewBatchFlushPayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return nil, nil, fmt.Errorf("unmarshal session review batch flush payload: %w", err)
		}
		return NewSessionReviewBatchFlushTask(p)
	})
}

func NewSessionReviewBatchFlushTask(payload SessionReviewBatchFlushPayload) (*asynq.Task, []asynq.Option, error) {
	if payload.QueueID == uuid.Nil {
		return nil, nil, fmt.Errorf("queue_id is required")
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal session review batch flush payload: %w", err)
	}
	return asynq.NewTask(TypeSessionReviewBatchFlush, encoded), []asynq.Option{
		asynq.Queue(QueueCritical),
		asynq.MaxRetry(5),
		asynq.Timeout(time.Minute),
		asynq.ProcessIn(githubReviewBatchWindow),
	}, nil
}

func EnqueueSessionReviewBatchFlush(ctx context.Context, enq enqueue.TaskEnqueuer, queueID uuid.UUID) error {
	if enq == nil {
		return nil
	}
	task, opts, err := NewSessionReviewBatchFlushTask(SessionReviewBatchFlushPayload{QueueID: queueID})
	if err != nil {
		return err
	}
	_, err = enq.EnqueueContext(ctx, task, opts...)
	return err
}

type SessionReviewBatchFlushHandler struct {
	db       *gorm.DB
	enqueuer enqueue.TaskEnqueuer
}

func NewSessionReviewBatchFlushHandler(db *gorm.DB, enqueuer enqueue.TaskEnqueuer) *SessionReviewBatchFlushHandler {
	return &SessionReviewBatchFlushHandler{db: db, enqueuer: enqueuer}
}

func (h *SessionReviewBatchFlushHandler) Handle(ctx context.Context, task *asynq.Task) error {
	var payload SessionReviewBatchFlushPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal session review batch flush payload: %w", err)
	}
	var sessionID uuid.UUID
	err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var queue model.SessionMessageQueue
		result := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND status = ?", payload.QueueID, reviewBatchBufferingStatus).
			Take(&queue)
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil
		}
		if result.Error != nil {
			return fmt.Errorf("load session review batch: %w", result.Error)
		}
		if err := tx.Model(&model.SessionMessageQueue{}).Where("id = ?", queue.ID).
			Updates(map[string]any{"status": "pending", "updated_at": time.Now().UTC()}).Error; err != nil {
			return fmt.Errorf("release session review batch: %w", err)
		}
		sessionID = queue.SessionID
		return nil
	})
	if err != nil || sessionID == uuid.Nil {
		return err
	}
	if err := EnqueueSessionMessageDeliver(ctx, h.enqueuer, sessionID); err != nil {
		restoreErr := h.db.WithContext(ctx).Model(&model.SessionMessageQueue{}).
			Where("id = ? AND status = ?", payload.QueueID, "pending").
			Updates(map[string]any{"status": reviewBatchBufferingStatus, "updated_at": time.Now().UTC()}).Error
		if restoreErr != nil {
			return errors.Join(
				fmt.Errorf("enqueue session review batch delivery: %w", err),
				fmt.Errorf("restore session review batch buffer: %w", restoreErr),
			)
		}
		return err
	}
	return nil
}
