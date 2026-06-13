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
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sandbox"
)

const employeeGitHubResourcesCloneTimeout = 5 * time.Minute

type EmployeeGitHubResourcesClonePayload struct {
	OrgID        uuid.UUID `json:"org_id"`
	EmployeeID   uuid.UUID `json:"employee_id"`
	ConnectionID uuid.UUID `json:"connection_id"`
}

func NewEmployeeGitHubResourcesCloneTask(payload EmployeeGitHubResourcesClonePayload) (*asynq.Task, []asynq.Option, error) {
	if payload.OrgID == uuid.Nil || payload.EmployeeID == uuid.Nil || payload.ConnectionID == uuid.Nil {
		return nil, nil, fmt.Errorf("github resources clone payload missing ids")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal github resources clone payload: %w", err)
	}
	opts := []asynq.Option{
		asynq.Queue(QueueDefault),
		asynq.MaxRetry(3),
		asynq.Timeout(employeeGitHubResourcesCloneTimeout),
		asynq.Unique(10 * time.Second),
		asynq.TaskID(fmt.Sprintf("employee-github-resources-clone:%s:%s", payload.EmployeeID, payload.ConnectionID)),
	}
	return asynq.NewTask(TypeEmployeeGitHubResourcesClone, body), opts, nil
}

type EmployeeGitHubResourcesCloneHandler struct {
	db           *gorm.DB
	orchestrator *sandbox.Orchestrator
	compileDeps  agentruntime.CompileDeps
}

func NewEmployeeGitHubResourcesCloneHandler(db *gorm.DB, orchestrator *sandbox.Orchestrator, compileDeps agentruntime.CompileDeps) *EmployeeGitHubResourcesCloneHandler {
	return &EmployeeGitHubResourcesCloneHandler{db: db, orchestrator: orchestrator, compileDeps: compileDeps}
}

func (h *EmployeeGitHubResourcesCloneHandler) Handle(ctx context.Context, task *asynq.Task) error {
	if h == nil || h.db == nil || h.orchestrator == nil || h.compileDeps.EncKey == nil {
		return fmt.Errorf("github resources clone handler not configured")
	}
	var payload EmployeeGitHubResourcesClonePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal github resources clone payload: %w", err)
	}
	fields := map[string]any{
		"org_id":        payload.OrgID.String(),
		"employee_id":   payload.EmployeeID.String(),
		"connection_id": payload.ConnectionID.String(),
	}
	if err := h.run(ctx, payload); err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("github resources clone failed: %w", err), fields)
		return err
	}
	logging.FromContext(ctx).InfoContext(ctx, "github resources clone completed",
		"org_id", payload.OrgID,
		"employee_id", payload.EmployeeID,
		"connection_id", payload.ConnectionID,
	)
	return nil
}

func (h *EmployeeGitHubResourcesCloneHandler) run(ctx context.Context, payload EmployeeGitHubResourcesClonePayload) error {
	var employee model.Employee
	if err := h.db.WithContext(ctx).
		Where("id = ? AND org_id = ? AND status <> ?", payload.EmployeeID, payload.OrgID, "archived").
		First(&employee).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return fmt.Errorf("load employee: %w", err)
	}
	var conn model.Connection
	if err := h.db.WithContext(ctx).Preload("Integration").
		Where("id = ? AND org_id = ? AND revoked_at IS NULL", payload.ConnectionID, payload.OrgID).
		First(&conn).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return fmt.Errorf("load github connection: %w", err)
	}
	sb, err := employeeRuntimeSelector(h.db, h.compileDeps).MainRuntime(ctx, payload.OrgID, payload.EmployeeID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return fmt.Errorf("load employee sandbox: %w", err)
	}
	if h.orchestrator.NeedsURLRefresh(sb) {
		if err := h.orchestrator.RefreshEmployeeSandboxURL(ctx, sb); err != nil {
			return fmt.Errorf("refresh employee sandbox url: %w", err)
		}
	}
	if err := h.orchestrator.SyncEmployeeSelectedRepositories(ctx, sb, &employee); err != nil {
		return err
	}
	if err := agentruntime.PushEmployeeRuntimeConfig(ctx, h.compileDeps, &employee, sb); err != nil {
		return fmt.Errorf("push employee runtime config: %w", err)
	}
	return nil
}
