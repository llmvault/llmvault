package handler

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/connectionaccess"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/tasks"
)

func enqueueGitHubRepositoryCloneForAlwaysOnAgents(ctx context.Context, db *gorm.DB, enq enqueue.TaskEnqueuer, orgID uuid.UUID, conn model.Connection) bool {
	if db == nil || enq == nil || orgID == uuid.Nil || conn.ID == uuid.Nil || !isGitHubProvider(conn.Integration.Provider) {
		return false
	}
	if selectedResourceCount(connectionDefaultResources(conn), "repository") == 0 {
		return false
	}
	var agentIDs []uuid.UUID
	if err := db.WithContext(ctx).
		Table("agent_plugin_installs").
		Joins("JOIN plugin_integrations ON plugin_integrations.plugin_id = agent_plugin_installs.plugin_id AND plugin_integrations.provider = ? AND plugin_integrations.kind = ?", conn.Integration.Provider, model.PluginIntegrationKindIntegration).
		Joins("JOIN agents ON agents.id = agent_plugin_installs.agent_id AND agents.org_id = agent_plugin_installs.org_id AND agents.status <> ? AND agents.sandbox_strategy = ?", "archived", agentStrategyAlwaysOn).
		Where("agent_plugin_installs.org_id = ?", orgID).
		Distinct("agent_plugin_installs.agent_id").
		Pluck("agent_plugin_installs.agent_id", &agentIDs).Error; err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("list always-on agents for github resource clone: %w", err), map[string]any{
			"org_id":        orgID.String(),
			"connection_id": conn.ID.String(),
		})
		return false
	}

	queued := false
	for _, agentID := range agentIDs {
		if enqueueGitHubRepositoryCloneTask(ctx, enq, orgID, agentID, conn.ID) {
			queued = true
		}
	}
	return queued
}

func enqueueGitHubRepositoryCloneForAgent(ctx context.Context, enq enqueue.TaskEnqueuer, agent model.Agent, conn model.Connection) bool {
	if enq == nil || agent.OrgID == nil || agent.SandboxStrategy != agentStrategyAlwaysOn || conn.ID == uuid.Nil || !isGitHubProvider(conn.Integration.Provider) {
		return false
	}
	if selectedResourceCount(connectionaccess.EffectiveResources(agent.Resources, conn), "repository") == 0 {
		return false
	}
	return enqueueGitHubRepositoryCloneTask(ctx, enq, *agent.OrgID, agent.ID, conn.ID)
}

func enqueueGitHubRepositoryCloneTask(ctx context.Context, enq enqueue.TaskEnqueuer, orgID, agentID, connID uuid.UUID) bool {
	task, opts, err := tasks.NewAgentGitHubResourcesCloneTask(tasks.AgentGitHubResourcesClonePayload{
		OrgID:        orgID,
		AgentID:      agentID,
		ConnectionID: connID,
	})
	if err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("create github resource clone task: %w", err), map[string]any{
			"agent_id":      agentID.String(),
			"connection_id": connID.String(),
		})
		return false
	}
	if _, err := enq.EnqueueContext(ctx, task, opts...); err != nil {
		if errors.Is(err, asynq.ErrDuplicateTask) || errors.Is(err, asynq.ErrTaskIDConflict) {
			return true
		}
		logging.CaptureWithFields(ctx, fmt.Errorf("enqueue github resource clone task: %w", err), map[string]any{
			"agent_id":      agentID.String(),
			"connection_id": connID.String(),
		})
		return false
	}
	return true
}

func connectionDefaultResources(conn model.Connection) model.JSON {
	raw, ok := conn.Meta["resources"]
	if !ok || raw == nil {
		return model.JSON{}
	}
	switch typed := raw.(type) {
	case model.JSON:
		return typed
	case map[string]any:
		out := model.JSON{}
		for key, value := range typed {
			out[key] = value
		}
		return out
	default:
		return model.JSON{}
	}
}

func selectedResourceCount(resources model.JSON, resourceType string) int {
	if len(resources) == 0 || resourceType == "" {
		return 0
	}
	raw, ok := resources[resourceType]
	if !ok || raw == nil {
		return 0
	}
	switch typed := raw.(type) {
	case []any:
		return len(typed)
	case []map[string]any:
		return len(typed)
	default:
		return 0
	}
}
