package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"github.com/usehivy/hivy/internal/enqueue"
)

type SessionReflectionPayload struct {
	SessionID uuid.UUID `json:"session_id"`
}

func init() {
	RegisterTaskBuilder(TypeSessionReflection, func(payload []byte) (*asynq.Task, []asynq.Option, error) {
		var p SessionReflectionPayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return nil, nil, fmt.Errorf("unmarshal session reflection payload: %w", err)
		}
		return NewSessionReflectionTask(p)
	})
}

func NewSessionReflectionTask(payload SessionReflectionPayload) (*asynq.Task, []asynq.Option, error) {
	if payload.SessionID == uuid.Nil {
		return nil, nil, fmt.Errorf("session_id is required")
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal session reflection payload: %w", err)
	}
	opts := []asynq.Option{
		asynq.Queue(QueueBulk),
		asynq.MaxRetry(3),
		asynq.Timeout(2 * time.Minute),
		asynq.Unique(2 * time.Minute),
	}
	return asynq.NewTask(TypeSessionReflection, encoded), opts, nil
}

func EnqueueSessionReflection(ctx context.Context, enq enqueue.TaskEnqueuer, sessionID uuid.UUID) error {
	if enq == nil {
		return nil
	}
	task, opts, err := NewSessionReflectionTask(SessionReflectionPayload{SessionID: sessionID})
	if err != nil {
		return err
	}
	if _, err := enq.EnqueueContext(ctx, task, opts...); err != nil {
		if errors.Is(err, asynq.ErrDuplicateTask) || errors.Is(err, asynq.ErrTaskIDConflict) {
			return nil
		}
		return err
	}
	return nil
}
