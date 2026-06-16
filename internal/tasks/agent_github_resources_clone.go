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

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/connectionaccess"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sandbox"
)

const agentGitHubResourcesCloneTimeout = 5 * time.Minute

type AgentGitHubResourcesClonePayload struct {
	OrgID        uuid.UUID `json:"org_id"`
	AgentID      uuid.UUID `json:"agent_id"`
	ConnectionID uuid.UUID `json:"connection_id"`
}

func NewAgentGitHubResourcesCloneTask(payload AgentGitHubResourcesClonePayload) (*asynq.Task, []asynq.Option, error) {
	if payload.OrgID == uuid.Nil || payload.AgentID == uuid.Nil || payload.ConnectionID == uuid.Nil {
		return nil, nil, fmt.Errorf("github resources clone payload missing ids")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal github resources clone payload: %w", err)
	}
	opts := []asynq.Option{
		asynq.Queue(QueueDefault),
		asynq.MaxRetry(3),
		asynq.Timeout(agentGitHubResourcesCloneTimeout),
		asynq.Unique(10 * time.Second),
		asynq.TaskID(fmt.Sprintf("agent-github-resources-clone:%s:%s", payload.AgentID, payload.ConnectionID)),
	}
	return asynq.NewTask(TypeAgentGitHubResourcesClone, body), opts, nil
}

type AgentGitHubResourcesCloneHandler struct {
	db           *gorm.DB
	orchestrator *sandbox.Orchestrator
	compileDeps  agentruntime.CompileDeps
}

func NewAgentGitHubResourcesCloneHandler(db *gorm.DB, orchestrator *sandbox.Orchestrator, compileDeps agentruntime.CompileDeps) *AgentGitHubResourcesCloneHandler {
	return &AgentGitHubResourcesCloneHandler{db: db, orchestrator: orchestrator, compileDeps: compileDeps}
}

func (h *AgentGitHubResourcesCloneHandler) Handle(ctx context.Context, task *asynq.Task) error {
	if h == nil || h.db == nil || h.orchestrator == nil || h.compileDeps.EncKey == nil {
		return fmt.Errorf("github resources clone handler not configured")
	}
	var payload AgentGitHubResourcesClonePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal github resources clone payload: %w", err)
	}
	fields := map[string]any{
		"org_id":        payload.OrgID.String(),
		"agent_id":      payload.AgentID.String(),
		"connection_id": payload.ConnectionID.String(),
	}
	if err := h.run(ctx, payload); err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("github resources clone failed: %w", err), fields)
		return err
	}
	logging.FromContext(ctx).InfoContext(ctx, "github resources clone completed",
		"org_id", payload.OrgID,
		"agent_id", payload.AgentID,
		"connection_id", payload.ConnectionID,
	)
	return nil
}

func (h *AgentGitHubResourcesCloneHandler) run(ctx context.Context, payload AgentGitHubResourcesClonePayload) error {
	var agent model.Agent
	if err := h.db.WithContext(ctx).
		Where("id = ? AND org_id = ? AND status <> ?", payload.AgentID, payload.OrgID, "archived").
		First(&agent).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return fmt.Errorf("load agent: %w", err)
	}
	access, err := connectionaccess.ResolveAgentConnection(ctx, h.db, payload.OrgID, payload.AgentID, payload.ConnectionID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return fmt.Errorf("resolve github connection access: %w", err)
	}
	if !connectionaccess.IsGitHubProvider(access.Connection.Integration.Provider) {
		return nil
	}
	sb, err := agentRuntimeSelector(h.db, h.compileDeps).MainRuntime(ctx, payload.OrgID, payload.AgentID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return fmt.Errorf("load agent sandbox: %w", err)
	}
	if h.orchestrator.NeedsURLRefresh(sb) {
		if err := h.orchestrator.RefreshAgentSandboxURL(ctx, sb); err != nil {
			return fmt.Errorf("refresh agent sandbox url: %w", err)
		}
	}
	if err := h.orchestrator.SyncAgentSelectedRepositories(ctx, sb, &agent); err != nil {
		return err
	}
	if err := agentruntime.PushAgentRuntimeConfig(ctx, h.compileDeps, &agent, sb); err != nil {
		return fmt.Errorf("push agent runtime config: %w", err)
	}
	return nil
}
