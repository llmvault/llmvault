package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sandbox"
)

type EmployeeSandboxRetireHandler struct {
	db           *gorm.DB
	orchestrator *sandbox.Orchestrator
}

func NewEmployeeSandboxRetireHandler(db *gorm.DB, orchestrator *sandbox.Orchestrator) *EmployeeSandboxRetireHandler {
	return &EmployeeSandboxRetireHandler{db: db, orchestrator: orchestrator}
}

func (h *EmployeeSandboxRetireHandler) Handle(ctx context.Context, task *asynq.Task) error {
	if h == nil || h.db == nil || h.orchestrator == nil {
		return fmt.Errorf("employee sandbox retire handler not configured")
	}
	var payload EmployeeSandboxRetirePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal employee sandbox retire payload: %w", err)
	}
	if payload.UpgradeID == uuid.Nil || payload.EmployeeID == uuid.Nil || payload.SandboxID == uuid.Nil {
		return fmt.Errorf("employee sandbox retire payload missing ids")
	}
	return h.retire(ctx, payload)
}

func (h *EmployeeSandboxRetireHandler) retire(ctx context.Context, payload EmployeeSandboxRetirePayload) error {
	var upgrade model.EmployeeSandboxUpgrade
	if err := h.db.WithContext(ctx).First(&upgrade, "id = ? AND employee_id = ?", payload.UpgradeID, payload.EmployeeID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return fmt.Errorf("load employee sandbox upgrade: %w", err)
	}
	if upgrade.Status != model.EmployeeSandboxUpgradeStatusSucceeded ||
		upgrade.Phase != model.EmployeeSandboxUpgradePhaseCompleted ||
		upgrade.OldSandboxID == nil ||
		*upgrade.OldSandboxID != payload.SandboxID {
		return nil
	}

	var sb model.Sandbox
	if err := h.db.WithContext(ctx).First(&sb, "id = ?", payload.SandboxID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return fmt.Errorf("load old employee sandbox: %w", err)
	}
	// The old sandbox is normally 'stopped' by Phase 6, but providers without a
	// pause primitive (Railway, which returns ErrUnsupported) leave it 'running'.
	// Either way the new sandbox is now serving traffic, so the old one must be
	// deleted to stop billing — retire from both states.
	if sb.EmployeeID == nil || *sb.EmployeeID != payload.EmployeeID ||
		(sb.Status != string(sandbox.StatusStopped) && sb.Status != string(sandbox.StatusRunning)) {
		return nil
	}
	if upgrade.NewSandboxID != nil && *upgrade.NewSandboxID == sb.ID {
		return nil
	}
	recordEmployeeSandboxRetire(ctx, &upgrade, &sb)
	logging.FromContext(ctx).InfoContext(ctx, "retiring old employee sandbox",
		"upgrade_id", upgrade.ID,
		"employee_id", payload.EmployeeID,
		"sandbox_id", sb.ID,
		"external_id", sb.ExternalID,
	)
	// Release the provider resource (the retire's billing goal) but KEEP the
	// control-plane sandbox row. A hard DeleteSandbox here would cascade-delete
	// the org's pre-upgrade conversation history — employee_session_events,
	// employee_sessions, specialist_tasks, employee_schedule_runs all FK the
	// sandbox ON DELETE CASCADE — which the upgrade is supposed to carry forward,
	// not destroy. DeleteSandboxResource marks the row 'archived' (excluded from
	// the runtime selector's active statuses) so it stops serving traffic while
	// the history it owns survives. Same blast-radius class as the
	// employee_schedules->sandboxes SET NULL fix.
	if err := h.orchestrator.DeleteSandboxResource(ctx, &sb); err != nil {
		return fmt.Errorf("retire employee sandbox resource: %w", err)
	}
	return nil
}
