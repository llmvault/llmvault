package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sandbox"
)

type AgentMemoryRefreshHandler struct {
	db           *gorm.DB
	orchestrator *sandbox.Orchestrator
	compileDeps  agentruntime.CompileDeps
}

func NewAgentMemoryRefreshHandler(db *gorm.DB, compileDeps agentruntime.CompileDeps, orchestrator ...*sandbox.Orchestrator) *AgentMemoryRefreshHandler {
	var orch *sandbox.Orchestrator
	if len(orchestrator) > 0 {
		orch = orchestrator[0]
	}
	return &AgentMemoryRefreshHandler{db: db, orchestrator: orch, compileDeps: compileDeps}
}

func (h *AgentMemoryRefreshHandler) Handle(ctx context.Context, task *asynq.Task) error {
	if h == nil || h.db == nil || h.compileDeps.EncKey == nil {
		logging.FromContext(ctx).WarnContext(ctx, "agent memory refresh skipped: handler dependencies missing")
		return nil
	}
	var payload AgentMemoryRefreshPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal agent memory refresh payload: %w", err)
	}
	fields := agentMemoryRefreshFields(payload)
	start := time.Now()
	logging.FromContext(ctx).InfoContext(ctx, "agent memory refresh started", fieldsToArgs(fields)...)
	if payload.AgentID == uuid.Nil {
		logging.FromContext(ctx).InfoContext(ctx, "agent memory refresh skipped",
			"skip_reason", "missing_agent_id",
			"reason", payload.Reason,
		)
		return nil
	}
	h.updateRefreshStatus(ctx, payload.AgentID, "running", "", nil)
	if err := h.refresh(ctx, payload); err != nil {
		h.updateRefreshStatus(ctx, payload.AgentID, "failed", err.Error(), nil)
		fields["duration_ms"] = time.Since(start).Milliseconds()
		logging.CaptureWithFields(ctx, fmt.Errorf("agent memory refresh failed: %w", err), fields)
		return err
	}
	now := time.Now().UTC()
	h.updateRefreshStatus(ctx, payload.AgentID, "succeeded", "", &now)
	fields["duration_ms"] = time.Since(start).Milliseconds()
	fields["last_memory_refreshed_at"] = now.Format(time.RFC3339Nano)
	logging.FromContext(ctx).InfoContext(ctx, "agent memory refresh completed", fieldsToArgs(fields)...)
	return nil
}

func (h *AgentMemoryRefreshHandler) refresh(ctx context.Context, payload AgentMemoryRefreshPayload) error {
	var agent model.Agent
	if err := h.db.WithContext(ctx).Where("id = ? AND status <> ?", payload.AgentID, "archived").First(&agent).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			logging.FromContext(ctx).InfoContext(ctx, "agent memory refresh skipped",
				"agent_id", payload.AgentID.String(),
				"sandbox_id", payload.SandboxID.String(),
				"reason", payload.Reason,
				"skip_reason", "agent_not_found_or_archived",
			)
			return nil
		}
		return fmt.Errorf("load agent for memory refresh: %w", err)
	}
	sb, err := h.loadSandbox(ctx, payload)
	if err != nil {
		return err
	}
	if sb == nil {
		logging.FromContext(ctx).InfoContext(ctx, "agent memory refresh skipped",
			"agent_id", payload.AgentID.String(),
			"sandbox_id", payload.SandboxID.String(),
			"reason", payload.Reason,
			"skip_reason", "sandbox_not_found",
		)
		return nil
	}
	fields := agentMemoryRefreshFields(payload)
	if agent.OrgID != nil {
		fields["org_id"] = agent.OrgID.String()
	}
	fields["resolved_sandbox_id"] = sb.ID.String()
	apiKey, err := h.compileDeps.EncKey.DecryptString(sb.EncryptedRuntimeSecret)
	if err != nil {
		return fmt.Errorf("decrypt agent runtime secret: %w", err)
	}
	configUpdate, _, err := agentruntime.BuildAgentRuntimeConfigUpdate(ctx, h.compileDeps, &agent, sb, apiKey)
	if err != nil {
		return fmt.Errorf("build agent runtime config for memory refresh: %w", err)
	}
	fields["memory_context_entries"] = runtimeMemoryContextEntryCount(configUpdate)
	fields["runtime_env_count"] = len(configUpdate.RuntimeEnv)
	client := agentruntime.NewClient(sb.RuntimeURL, apiKey)
	if h.orchestrator != nil {
		var err error
		client, err = h.orchestrator.GetRuntimeClient(ctx, sb)
		if err != nil {
			return fmt.Errorf("agent runtime client: %w", err)
		}
	} else if err := client.Healthz(ctx); err != nil {
		return fmt.Errorf("agent runtime healthz: %w", err)
	}
	if _, err := client.PutRuntimeConfig(ctx, configUpdate); err != nil {
		return fmt.Errorf("agent runtime put config: %w", err)
	}
	if err := client.Readyz(ctx); err != nil {
		return fmt.Errorf("agent runtime readyz: %w", err)
	}
	logging.FromContext(ctx).InfoContext(ctx, "agent memory refreshed",
		"agent_id", agent.ID,
		"sandbox_id", sb.ID,
		"reason", payload.Reason,
		"memory_context_entries", fields["memory_context_entries"],
		"runtime_env_count", fields["runtime_env_count"],
	)
	return nil
}

func (h *AgentMemoryRefreshHandler) updateRefreshStatus(ctx context.Context, agentID uuid.UUID, status, message string, refreshedAt *time.Time) {
	if h == nil || h.db == nil || agentID == uuid.Nil {
		return
	}
	updates := map[string]any{
		"memory_refresh_status": status,
		"memory_refresh_error":  truncateMemoryRefreshError(message),
	}
	if refreshedAt != nil {
		updates["last_memory_refreshed_at"] = *refreshedAt
	}
	if err := h.db.WithContext(ctx).Model(&model.Agent{}).Where("id = ?", agentID).Updates(updates).Error; err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("agent memory refresh: update status: %w", err), map[string]any{
			"agent_id": agentID.String(),
			"status":   status,
		})
	} else {
		logging.FromContext(ctx).InfoContext(ctx, "agent memory refresh status updated",
			"agent_id", agentID.String(),
			"status", status,
			"has_error", strings.TrimSpace(message) != "",
		)
	}
}

func truncateMemoryRefreshError(message string) string {
	message = strings.TrimSpace(message)
	if len(message) <= 2000 {
		return message
	}
	return message[:2000]
}

func (h *AgentMemoryRefreshHandler) loadSandbox(ctx context.Context, payload AgentMemoryRefreshPayload) (*model.Sandbox, error) {
	var agent model.Agent
	if err := h.db.WithContext(ctx).Select("org_id").First(&agent, "id = ?", payload.AgentID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("load agent org for memory refresh: %w", err)
	}
	if agent.OrgID == nil {
		return nil, nil
	}
	selector := agentRuntimeSelector(h.db, h.compileDeps)
	var err error
	var sb *model.Sandbox
	if payload.SandboxID != uuid.Nil {
		sb, err = selector.MainRuntimeByID(ctx, *agent.OrgID, payload.AgentID, payload.SandboxID)
	} else {
		sb, err = selector.MainRuntime(ctx, *agent.OrgID, payload.AgentID)
	}
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("load agent sandbox for memory refresh: %w", err)
	}
	return sb, nil
}

func agentMemoryRefreshFields(payload AgentMemoryRefreshPayload) map[string]any {
	return map[string]any{
		"agent_id":   payload.AgentID.String(),
		"sandbox_id": payload.SandboxID.String(),
		"reason":     strings.TrimSpace(payload.Reason),
	}
}

func runtimeMemoryContextEntryCount(req agentruntime.ConfigUpdateRequest) int {
	if req.Definition == nil || req.Definition.Context == nil {
		return 0
	}
	switch value := req.Definition.Context["memory"].(type) {
	case agentruntime.MemoryContext:
		return len(value.Entries)
	case *agentruntime.MemoryContext:
		if value == nil {
			return 0
		}
		return len(value.Entries)
	default:
		return 0
	}
}
