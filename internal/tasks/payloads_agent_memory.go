package tasks

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

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
