package handler

import (
	"errors"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"github.com/usehivy/hivy/internal/tasks"
)

func (h *AgentHandler) deleteStaleAgentSandboxUpgradeTask(agentID uuid.UUID) error {
	if h.taskCleaner == nil {
		return nil
	}
	err := h.taskCleaner.DeleteTask(tasks.QueueBulk, tasks.AgentSandboxUpgradeTaskID(agentID))
	if errors.Is(err, asynq.ErrTaskNotFound) || errors.Is(err, asynq.ErrQueueNotFound) {
		return nil
	}
	return err
}
