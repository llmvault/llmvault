package tasks

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

const agentSandboxUpgradeTimeout = 90 * time.Minute
const agentSandboxRetireDelay = 24 * time.Hour

type AgentSandboxUpgradePayload struct {
	UpgradeID uuid.UUID `json:"upgrade_id"`
	AgentID   uuid.UUID `json:"agent_id"`
}

type AgentSandboxRetirePayload struct {
	UpgradeID uuid.UUID `json:"upgrade_id"`
	AgentID   uuid.UUID `json:"agent_id"`
	SandboxID uuid.UUID `json:"sandbox_id"`
}

func NewAgentSandboxUpgradeTask(upgradeID, agentID uuid.UUID) (*asynq.Task, []asynq.Option, error) {
	payload, err := json.Marshal(AgentSandboxUpgradePayload{
		UpgradeID: upgradeID,
		AgentID:   agentID,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("marshal agent sandbox upgrade payload: %w", err)
	}
	opts := []asynq.Option{
		asynq.Queue(QueueBulk),
		asynq.MaxRetry(0),
		asynq.Timeout(agentSandboxUpgradeTimeout),
		asynq.TaskID(AgentSandboxUpgradeTaskID(agentID)),
	}
	return asynq.NewTask(TypeAgentSandboxUpgrade, payload), opts, nil
}

func AgentSandboxUpgradeTaskID(agentID uuid.UUID) string {
	return "agent-sandbox-upgrade:" + agentID.String()
}

func NewAgentSandboxRetireTask(payload AgentSandboxRetirePayload) (*asynq.Task, []asynq.Option, error) {
	if payload.UpgradeID == uuid.Nil || payload.AgentID == uuid.Nil || payload.SandboxID == uuid.Nil {
		return nil, nil, fmt.Errorf("agent sandbox retire payload missing ids")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal agent sandbox retire payload: %w", err)
	}
	opts := []asynq.Option{
		asynq.Queue(QueueDefault),
		asynq.MaxRetry(3),
		asynq.Timeout(2 * time.Minute),
		asynq.ProcessIn(agentSandboxRetireDelay),
		asynq.TaskID("agent-sandbox-retire:" + payload.SandboxID.String()),
	}
	return asynq.NewTask(TypeAgentSandboxRetire, body), opts, nil
}
