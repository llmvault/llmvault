package tasks

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/model"
)

type FailedEventInput struct {
	OrgID        uuid.UUID
	TriggerID    uuid.UUID
	EventType    string
	Payload      []byte
	Err          error
	AttemptCount int
}

func PersistTerminalFailure(ctx context.Context, db *gorm.DB, in FailedEventInput) error {
	if in.Err == nil {
		return errors.New("failed_events: nil error")
	}
	row := model.FailedEvent{
		OrgID:        in.OrgID,
		TriggerID:    in.TriggerID,
		EventType:    in.EventType,
		Payload:      model.RawJSON(in.Payload),
		Error:        in.Err.Error(),
		AttemptCount: in.AttemptCount,
		FailedAt:     time.Now().UTC(),
		Status:       model.FailedEventStatusPending,
	}
	if err := db.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("insert failed_event: %w", err)
	}
	return nil
}

// TaskBuilder rebuilds a task (and its enqueue options) from a stored payload.
// Options are returned separately so the failed-event retry path re-applies the
// task's original Queue/MaxRetry/Timeout instead of silently falling back to
// asynq defaults (see P0-11).
type TaskBuilder func(payload []byte) (*asynq.Task, []asynq.Option, error)

var (
	taskBuildersMu sync.RWMutex
	taskBuilders   = map[string]TaskBuilder{}
)

func RegisterTaskBuilder(eventType string, fn TaskBuilder) {
	taskBuildersMu.Lock()
	defer taskBuildersMu.Unlock()
	taskBuilders[eventType] = fn
}

func lookupTaskBuilder(eventType string) (TaskBuilder, bool) {
	taskBuildersMu.RLock()
	defer taskBuildersMu.RUnlock()
	fn, ok := taskBuilders[eventType]
	return fn, ok
}

var ErrFailedEventNotPending = errors.New("failed_events: row is not pending")

func RetryFailedEvent(ctx context.Context, db *gorm.DB, enqueuer enqueue.TaskEnqueuer, id uuid.UUID) (*asynq.TaskInfo, error) {
	var row model.FailedEvent
	if err := db.WithContext(ctx).Where("id = ?", id).First(&row).Error; err != nil {
		return nil, fmt.Errorf("load failed_event %s: %w", id, err)
	}
	builder, ok := lookupTaskBuilder(row.EventType)
	if !ok {
		return nil, fmt.Errorf("no task builder registered for event_type %q", row.EventType)
	}
	task, opts, err := builder([]byte(row.Payload))
	if err != nil {
		return nil, fmt.Errorf("build task for retry: %w", err)
	}

	// CAS the status to "retried" BEFORE enqueueing so two concurrent retries
	// can't both pass a read-time pending check and enqueue twice (P2-42).
	// The conditional UPDATE only succeeds for exactly one caller; the loser
	// sees RowsAffected == 0 and bails with ErrFailedEventNotPending, mirroring
	// DiscardFailedEvent's claim-then-act pattern.
	now := time.Now().UTC()
	claim := db.WithContext(ctx).Model(&model.FailedEvent{}).
		Where("id = ? AND status = ?", row.ID, model.FailedEventStatusPending).
		Updates(map[string]any{
			"status":     model.FailedEventStatusRetried,
			"retried_at": now,
		})
	if claim.Error != nil {
		return nil, fmt.Errorf("claim failed_event for retry: %w", claim.Error)
	}
	if claim.RowsAffected == 0 {
		return nil, ErrFailedEventNotPending
	}

	info, err := enqueuer.EnqueueContext(ctx, task, opts...)
	if err != nil {
		// Roll the claim back to pending so the retry can be re-attempted; a
		// permanent "retried" row with no enqueued task would strand the event.
		if rbErr := db.WithContext(ctx).Model(&model.FailedEvent{}).
			Where("id = ?", row.ID).
			Updates(map[string]any{
				"status":     model.FailedEventStatusPending,
				"retried_at": nil,
			}).Error; rbErr != nil {
			return nil, fmt.Errorf("enqueue retry: %w (and rollback failed: %v)", err, rbErr)
		}
		return nil, fmt.Errorf("enqueue retry: %w", err)
	}

	if err := db.WithContext(ctx).Model(&model.FailedEvent{}).
		Where("id = ?", row.ID).
		Update("retried_task_id", info.ID).Error; err != nil {
		// The task is enqueued and the row is already marked retried; only the
		// task-id bookkeeping failed. Surface it but return the info.
		return info, fmt.Errorf("record retried_task_id: %w", err)
	}
	return info, nil
}

func DiscardFailedEvent(ctx context.Context, db *gorm.DB, id uuid.UUID) error {
	res := db.WithContext(ctx).Model(&model.FailedEvent{}).
		Where("id = ? AND status = ?", id, model.FailedEventStatusPending).
		Update("status", model.FailedEventStatusDiscarded)
	if res.Error != nil {
		return fmt.Errorf("discard failed_event: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrFailedEventNotPending
	}
	return nil
}
