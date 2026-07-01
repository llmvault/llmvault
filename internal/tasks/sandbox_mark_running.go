package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sandbox"
)

type SandboxMarkRunningPayload struct {
	SandboxID uuid.UUID `json:"sandbox_id"`
}

func NewSandboxMarkRunningTask(payload SandboxMarkRunningPayload) (*asynq.Task, []asynq.Option, error) {
	if payload.SandboxID == uuid.Nil {
		return nil, nil, fmt.Errorf("sandbox_id is required")
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal sandbox mark running payload: %w", err)
	}
	opts := []asynq.Option{
		asynq.Queue(QueueDefault),
		asynq.MaxRetry(3),
		asynq.Timeout(30 * time.Second),
		// Collapse websocket reconnect flapping to one flip per sandbox per window.
		asynq.Unique(time.Minute),
	}
	return asynq.NewTask(TypeSandboxMarkRunning, encoded), opts, nil
}

// EnqueueSandboxMarkRunning schedules a flip of the sandbox's status back to
// 'running'. Safe to call with a nil enqueuer (no-op).
func EnqueueSandboxMarkRunning(ctx context.Context, enq enqueue.TaskEnqueuer, sandboxID uuid.UUID) error {
	if enq == nil {
		return nil
	}
	task, opts, err := NewSandboxMarkRunningTask(SandboxMarkRunningPayload{SandboxID: sandboxID})
	if err != nil {
		return err
	}
	if _, err := enq.EnqueueContext(ctx, task, opts...); err != nil {
		return err
	}
	return nil
}

type SandboxMarkRunningHandler struct {
	db *gorm.DB
}

func NewSandboxMarkRunningHandler(db *gorm.DB) *SandboxMarkRunningHandler {
	return &SandboxMarkRunningHandler{db: db}
}

func (h *SandboxMarkRunningHandler) Handle(ctx context.Context, task *asynq.Task) error {
	var payload SandboxMarkRunningPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal payload: %w", err)
	}
	// Only lift a slept sandbox; never clobber archived/error/other statuses.
	return h.db.WithContext(ctx).Model(&model.Sandbox{}).
		Where("id = ? AND status = ?", payload.SandboxID, string(sandbox.StatusStopped)).
		Update("status", string(sandbox.StatusRunning)).Error
}
