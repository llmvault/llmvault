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

	"github.com/usehivy/hivy/internal/employeeruntime"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

type EmployeeMemoryRefreshHandler struct {
	db          *gorm.DB
	compileDeps employeeruntime.CompileDeps
}

func NewEmployeeMemoryRefreshHandler(db *gorm.DB, compileDeps employeeruntime.CompileDeps) *EmployeeMemoryRefreshHandler {
	return &EmployeeMemoryRefreshHandler{db: db, compileDeps: compileDeps}
}

func (h *EmployeeMemoryRefreshHandler) Handle(ctx context.Context, task *asynq.Task) error {
	if h == nil || h.db == nil || h.compileDeps.EncKey == nil {
		logging.FromContext(ctx).WarnContext(ctx, "employee memory refresh skipped: handler dependencies missing")
		return nil
	}
	var payload EmployeeMemoryRefreshPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal employee memory refresh payload: %w", err)
	}
	fields := employeeMemoryRefreshFields(payload)
	start := time.Now()
	logging.FromContext(ctx).InfoContext(ctx, "employee memory refresh started", fieldsToArgs(fields)...)
	if payload.EmployeeID == uuid.Nil {
		logging.FromContext(ctx).InfoContext(ctx, "employee memory refresh skipped",
			"skip_reason", "missing_employee_id",
			"reason", payload.Reason,
		)
		return nil
	}
	h.updateRefreshStatus(ctx, payload.EmployeeID, "running", "", nil)
	if err := h.refresh(ctx, payload); err != nil {
		h.updateRefreshStatus(ctx, payload.EmployeeID, "failed", err.Error(), nil)
		fields["duration_ms"] = time.Since(start).Milliseconds()
		logging.CaptureWithFields(ctx, fmt.Errorf("employee memory refresh failed: %w", err), fields)
		return err
	}
	now := time.Now().UTC()
	h.updateRefreshStatus(ctx, payload.EmployeeID, "succeeded", "", &now)
	fields["duration_ms"] = time.Since(start).Milliseconds()
	fields["last_memory_refreshed_at"] = now.Format(time.RFC3339Nano)
	logging.FromContext(ctx).InfoContext(ctx, "employee memory refresh completed", fieldsToArgs(fields)...)
	return nil
}

func (h *EmployeeMemoryRefreshHandler) refresh(ctx context.Context, payload EmployeeMemoryRefreshPayload) error {
	var agent model.Employee
	if err := h.db.WithContext(ctx).Where("id = ? AND status <> ?", payload.EmployeeID, "archived").First(&agent).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			logging.FromContext(ctx).InfoContext(ctx, "employee memory refresh skipped",
				"employee_id", payload.EmployeeID.String(),
				"sandbox_id", payload.SandboxID.String(),
				"reason", payload.Reason,
				"skip_reason", "employee_not_found_or_archived",
			)
			return nil
		}
		return fmt.Errorf("load employee for memory refresh: %w", err)
	}
	sb, err := h.loadSandbox(ctx, payload)
	if err != nil {
		return err
	}
	if sb == nil {
		logging.FromContext(ctx).InfoContext(ctx, "employee memory refresh skipped",
			"employee_id", payload.EmployeeID.String(),
			"sandbox_id", payload.SandboxID.String(),
			"reason", payload.Reason,
			"skip_reason", "sandbox_not_found",
		)
		return nil
	}
	fields := employeeMemoryRefreshFields(payload)
	if agent.OrgID != nil {
		fields["org_id"] = agent.OrgID.String()
	}
	fields["resolved_sandbox_id"] = sb.ID.String()
	apiKey, err := h.compileDeps.EncKey.DecryptString(sb.EncryptedRuntimeSecret)
	if err != nil {
		return fmt.Errorf("decrypt employee runtime secret: %w", err)
	}
	configUpdate, _, err := employeeruntime.BuildEmployeeRuntimeConfigUpdate(ctx, h.compileDeps, &agent, sb, apiKey)
	if err != nil {
		return fmt.Errorf("build employee runtime config for memory refresh: %w", err)
	}
	fields["memory_context_entries"] = runtimeMemoryContextEntryCount(configUpdate)
	fields["runtime_env_count"] = len(configUpdate.RuntimeEnv)
	client := employeeruntime.NewClient(sb.RuntimeURL, apiKey)
	if err := client.Healthz(ctx); err != nil {
		return fmt.Errorf("employee runtime healthz: %w", err)
	}
	if _, err := client.PutRuntimeConfig(ctx, configUpdate); err != nil {
		return fmt.Errorf("employee runtime put config: %w", err)
	}
	if err := client.Readyz(ctx); err != nil {
		return fmt.Errorf("employee runtime readyz: %w", err)
	}
	logging.FromContext(ctx).InfoContext(ctx, "employee memory refreshed",
		"employee_id", agent.ID,
		"sandbox_id", sb.ID,
		"reason", payload.Reason,
		"memory_context_entries", fields["memory_context_entries"],
		"runtime_env_count", fields["runtime_env_count"],
	)
	return nil
}

func (h *EmployeeMemoryRefreshHandler) updateRefreshStatus(ctx context.Context, agentID uuid.UUID, status, message string, refreshedAt *time.Time) {
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
	if err := h.db.WithContext(ctx).Model(&model.Employee{}).Where("id = ?", agentID).Updates(updates).Error; err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("employee memory refresh: update status: %w", err), map[string]any{
			"employee_id": agentID.String(),
			"status":      status,
		})
	} else {
		logging.FromContext(ctx).InfoContext(ctx, "employee memory refresh status updated",
			"employee_id", agentID.String(),
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

func (h *EmployeeMemoryRefreshHandler) loadSandbox(ctx context.Context, payload EmployeeMemoryRefreshPayload) (*model.Sandbox, error) {
	var agent model.Employee
	if err := h.db.WithContext(ctx).Select("org_id").First(&agent, "id = ?", payload.EmployeeID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("load employee org for memory refresh: %w", err)
	}
	if agent.OrgID == nil {
		return nil, nil
	}
	selector := employeeRuntimeSelector(h.db, h.compileDeps)
	var err error
	var sb *model.Sandbox
	if payload.SandboxID != uuid.Nil {
		sb, err = selector.MainRuntimeByID(ctx, *agent.OrgID, payload.EmployeeID, payload.SandboxID)
	} else {
		sb, err = selector.MainRuntime(ctx, *agent.OrgID, payload.EmployeeID)
	}
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("load employee sandbox for memory refresh: %w", err)
	}
	return sb, nil
}

func employeeMemoryRefreshFields(payload EmployeeMemoryRefreshPayload) map[string]any {
	return map[string]any{
		"employee_id": payload.EmployeeID.String(),
		"sandbox_id":  payload.SandboxID.String(),
		"reason":      strings.TrimSpace(payload.Reason),
	}
}

func runtimeMemoryContextEntryCount(req employeeruntime.ConfigUpdateRequest) int {
	if req.Definition == nil || req.Definition.Context == nil {
		return 0
	}
	switch value := req.Definition.Context["memory"].(type) {
	case employeeruntime.MemoryContext:
		return len(value.Entries)
	case *employeeruntime.MemoryContext:
		if value == nil {
			return 0
		}
		return len(value.Entries)
	default:
		return 0
	}
}
