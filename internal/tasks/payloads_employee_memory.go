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

const EmployeeMemoryRetainDelay = 10 * time.Minute

type EmployeeMemoryRetainPayload struct {
	EmployeeID        uuid.UUID `json:"employee_id"`
	SandboxID         uuid.UUID `json:"sandbox_id"`
	EmployeeSessionID uuid.UUID `json:"employee_session_id,omitempty"`
	SessionID         string    `json:"session_id"`
	Reason            string    `json:"reason,omitempty"`
	SourceEvent       string    `json:"source_event,omitempty"`
}

// NewEmployeeMemoryRetainTask creates a task that retains employee memory.
// Options are returned separately so they survive the enqueue client's Sentry
// trace-payload rewrite (P0-11).
func NewEmployeeMemoryRetainTask(payload EmployeeMemoryRetainPayload) (*asynq.Task, []asynq.Option, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal employee memory retain payload: %w", err)
	}
	opts := []asynq.Option{
		asynq.Queue(QueueDefault),
		asynq.MaxRetry(3),
		asynq.Timeout(4 * time.Minute),
	}
	return asynq.NewTask(TypeEmployeeMemoryRetain, body), opts, nil
}

func EnqueueEmployeeMemoryRetain(ctx context.Context, enqueuer enqueue.TaskEnqueuer, payload EmployeeMemoryRetainPayload) (bool, error) {
	if enqueuer == nil {
		return false, fmt.Errorf("memory retain enqueuer missing")
	}
	task, opts, err := NewEmployeeMemoryRetainTask(payload)
	if err != nil {
		return false, err
	}
	opts = append(opts,
		asynq.ProcessIn(EmployeeMemoryRetainDelay),
		asynq.TaskID(EmployeeMemoryRetainTaskID(payload)),
	)
	_, err = enqueuer.EnqueueContext(ctx, task, opts...)
	if errors.Is(err, asynq.ErrDuplicateTask) {
		return true, nil
	}
	return false, err
}

func EmployeeMemoryRetainTaskID(payload EmployeeMemoryRetainPayload) string {
	if payload.EmployeeSessionID != uuid.Nil {
		return "employee-memory-retain:" + payload.EmployeeSessionID.String()
	}
	return "employee-memory-retain:" + payload.SandboxID.String() + ":" + payload.SessionID
}

type EmployeeMemoryRefreshPayload struct {
	EmployeeID uuid.UUID `json:"employee_id"`
	SandboxID  uuid.UUID `json:"sandbox_id,omitempty"`
	Reason     string    `json:"reason,omitempty"`
}

// NewEmployeeMemoryRefreshTask creates a task that refreshes employee memory.
// Options are returned separately so they survive the enqueue client's Sentry
// trace-payload rewrite (P0-11).
func NewEmployeeMemoryRefreshTask(payload EmployeeMemoryRefreshPayload) (*asynq.Task, []asynq.Option, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal employee memory refresh payload: %w", err)
	}
	opts := []asynq.Option{
		asynq.Queue(QueueDefault),
		asynq.MaxRetry(3),
		asynq.Timeout(2 * time.Minute),
	}
	return asynq.NewTask(TypeEmployeeMemoryRefresh, body), opts, nil
}
