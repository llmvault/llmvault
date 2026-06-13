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

const AgentMemoryRetainDelay = 10 * time.Minute

type AgentMemoryRetainPayload struct {
	AgentID     uuid.UUID `json:"agent_id"`
	SandboxID   uuid.UUID `json:"sandbox_id"`
	SessionUUID uuid.UUID `json:"session_uuid,omitempty"`
	SessionID   string    `json:"session_id"`
	Reason      string    `json:"reason,omitempty"`
	SourceEvent string    `json:"source_event,omitempty"`
}

// NewAgentMemoryRetainTask creates a task that retains agent memory.
// Options are returned separately (see WebhookForwardPayload's NewWebhookForwardTask).
func NewAgentMemoryRetainTask(payload AgentMemoryRetainPayload) (*asynq.Task, []asynq.Option, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal agent memory retain payload: %w", err)
	}
	opts := []asynq.Option{
		asynq.Queue(QueueDefault),
		asynq.MaxRetry(3),
		asynq.Timeout(4 * time.Minute),
	}
	return asynq.NewTask(TypeAgentMemoryRetain, body), opts, nil
}

func EnqueueAgentMemoryRetain(ctx context.Context, enqueuer enqueue.TaskEnqueuer, payload AgentMemoryRetainPayload) (bool, error) {
	if enqueuer == nil {
		return false, fmt.Errorf("memory retain enqueuer missing")
	}
	task, opts, err := NewAgentMemoryRetainTask(payload)
	if err != nil {
		return false, err
	}
	opts = append(opts,
		asynq.ProcessIn(AgentMemoryRetainDelay),
		asynq.TaskID(AgentMemoryRetainTaskID(payload)),
	)
	_, err = enqueuer.EnqueueContext(ctx, task, opts...)
	if errors.Is(err, asynq.ErrDuplicateTask) {
		return true, nil
	}
	return false, err
}

func AgentMemoryRetainTaskID(payload AgentMemoryRetainPayload) string {
	if payload.SessionUUID != uuid.Nil {
		return "agent-memory-retain:" + payload.SessionUUID.String()
	}
	return "agent-memory-retain:" + payload.SandboxID.String() + ":" + payload.SessionID
}

type AgentMemoryRefreshPayload struct {
	AgentID   uuid.UUID `json:"agent_id"`
	SandboxID uuid.UUID `json:"sandbox_id,omitempty"`
	Reason    string    `json:"reason,omitempty"`
}

// NewAgentMemoryRefreshTask creates a task that refreshes agent memory.
// Options are returned separately (see WebhookForwardPayload's NewWebhookForwardTask).
func NewAgentMemoryRefreshTask(payload AgentMemoryRefreshPayload) (*asynq.Task, []asynq.Option, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal agent memory refresh payload: %w", err)
	}
	opts := []asynq.Option{
		asynq.Queue(QueueDefault),
		asynq.MaxRetry(3),
		asynq.Timeout(2 * time.Minute),
	}
	return asynq.NewTask(TypeAgentMemoryRefresh, body), opts, nil
}
