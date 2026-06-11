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

	"github.com/usehivy/hivy/internal/employeeruntime"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sandbox"
	"github.com/usehivy/hivy/internal/storage"
)

const employeeSandboxUpgradeCommandTimeout = 60 * time.Minute

type employeeSandboxUpgradeBackupStore interface {
	Head(ctx context.Context, key string) (*storage.S3ObjectInfo, error)
	PresignedURL(ctx context.Context, key string, ttl time.Duration) (string, error)
	PresignedPutURL(ctx context.Context, key string, ttl time.Duration) (string, error)
}

type EmployeeSandboxUpgradeHandler struct {
	db           *gorm.DB
	orchestrator *sandbox.Orchestrator
	store        employeeSandboxUpgradeBackupStore
	compileDeps  employeeruntime.CompileDeps
	enqueuer     enqueue.TaskEnqueuer
}

func NewEmployeeSandboxUpgradeHandler(
	db *gorm.DB,
	orchestrator *sandbox.Orchestrator,
	store employeeSandboxUpgradeBackupStore,
	compileDeps employeeruntime.CompileDeps,
	enqueuer enqueue.TaskEnqueuer,
) *EmployeeSandboxUpgradeHandler {
	return &EmployeeSandboxUpgradeHandler{
		db:           db,
		orchestrator: orchestrator,
		store:        store,
		compileDeps:  compileDeps,
		enqueuer:     enqueuer,
	}
}

func (h *EmployeeSandboxUpgradeHandler) Handle(ctx context.Context, task *asynq.Task) error {
	if h == nil || h.db == nil || h.orchestrator == nil || h.store == nil || h.compileDeps.EncKey == nil || h.enqueuer == nil {
		return fmt.Errorf("employee sandbox upgrade handler not configured")
	}
	var payload EmployeeSandboxUpgradePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal employee sandbox upgrade payload: %w", err)
	}
	if payload.UpgradeID == uuid.Nil || payload.EmployeeID == uuid.Nil {
		return fmt.Errorf("employee sandbox upgrade payload missing ids")
	}
	return h.run(ctx, payload)
}

func (h *EmployeeSandboxUpgradeHandler) run(ctx context.Context, payload EmployeeSandboxUpgradePayload) error {
	log := logging.FromContext(ctx)

	upgrade, agent, oldSandbox, err := h.loadAndStart(ctx, payload)
	if err != nil {
		return err
	}
	if upgrade == nil {
		return nil
	}
	annotateEmployeeSandboxUpgradeSentry(ctx, upgrade, agent, oldSandbox)

	var oldPaused bool
	var newSandbox *model.Sandbox
	fail := func(phase string, cause error) error {
		msg := cause.Error()
		// Rollback must complete even when the task context is already cancelled
		// (asynq deadline, worker drain). A cancelled ctx would no-op the provider
		// delete and DB writes below, stranding both the new and old sandboxes.
		rollbackCtx := context.WithoutCancel(ctx)
		recordEmployeeSandboxUpgradeFailure(rollbackCtx, upgrade, phase, msg)
		log.ErrorContext(ctx, "employee sandbox upgrade failed",
			"upgrade_id", upgrade.ID,
			"employee_id", upgrade.EmployeeID,
			"phase", phase,
			"error", msg,
		)
		if newSandbox != nil && newSandbox.ID != uuid.Nil {
			if err := h.orchestrator.DeleteSandbox(rollbackCtx, newSandbox); err != nil {
				msg += "; failed to delete new sandbox during rollback: " + err.Error()
			}
		}
		if oldSandbox != nil && oldSandbox.ID != uuid.Nil {
			if oldPaused {
				if err := h.orchestrator.StartEmployeeSandbox(rollbackCtx, oldSandbox); err != nil {
					msg += "; failed to restart old sandbox during rollback: " + err.Error()
				} else if err := h.syncEmployeeRuntime(rollbackCtx, agent, oldSandbox); err != nil {
					msg += "; failed to sync old sandbox during rollback: " + err.Error()
				}
			} else {
				_ = h.db.WithContext(rollbackCtx).Model(oldSandbox).Updates(map[string]any{
					"status":        string(sandbox.StatusRunning),
					"error_message": nil,
				}).Error
				oldSandbox.Status = string(sandbox.StatusRunning)
				oldSandbox.ErrorMessage = nil
			}
		}
		h.markFailed(rollbackCtx, upgrade, phase, msg)
		return cause
	}

	// Phase 1: Snapshot the old runtime while it is still available.
	if err := h.markPhase(ctx, upgrade, model.EmployeeSandboxUpgradePhaseBackup); err != nil {
		return err
	}
	backupMeta, err := h.runBackup(ctx, upgrade, agent, oldSandbox)
	if err != nil {
		return fail(model.EmployeeSandboxUpgradePhaseBackup, err)
	}
	if err := h.verifyAndRecordBackup(ctx, upgrade, backupMeta); err != nil {
		return fail(model.EmployeeSandboxUpgradePhaseBackup, err)
	}
	recordEmployeeSandboxUpgradeBackup(ctx, upgrade, backupMeta)

	// Phase 2: Create new sandbox.
	if err := h.markPhase(ctx, upgrade, model.EmployeeSandboxUpgradePhaseCreatingNew); err != nil {
		return err
	}
	secrets, err := employeeruntime.PrepareStartup(ctx, h.compileDeps, agent)
	if err != nil {
		return fail(model.EmployeeSandboxUpgradePhaseCreatingNew, err)
	}
	newSandbox, err = h.orchestrator.CreateEmployeeSandbox(ctx, agent, secrets)
	if err != nil {
		return fail(model.EmployeeSandboxUpgradePhaseCreatingNew, err)
	}
	// CreateEmployeeSandbox marks the new sandbox 'running'. The runtime selector
	// orders main runtimes by created_at DESC, so a running new sandbox would be
	// picked for live traffic immediately — before the old session DB is restored
	// (Phase 3) and synced (Phase 5). Park it in the non-selectable 'upgrading'
	// status so turns keep landing on the old sandbox until the new one is ready.
	if err := h.db.WithContext(ctx).Model(newSandbox).Update("status", string(sandbox.StatusUpgrading)).Error; err != nil {
		return fail(model.EmployeeSandboxUpgradePhaseCreatingNew, fmt.Errorf("park new sandbox as upgrading: %w", err))
	}
	newSandbox.Status = string(sandbox.StatusUpgrading)
	if err := h.db.WithContext(ctx).Model(upgrade).Update("new_sandbox_id", newSandbox.ID).Error; err != nil {
		return fail(model.EmployeeSandboxUpgradePhaseCreatingNew, err)
	}
	upgrade.NewSandboxID = &newSandbox.ID
	recordEmployeeSandboxUpgradeNewSandbox(ctx, upgrade, newSandbox)

	// Phase 3: Restore the verified SQLite snapshot into the new runtime.
	if err := h.markPhase(ctx, upgrade, model.EmployeeSandboxUpgradePhaseRestore); err != nil {
		return fail(model.EmployeeSandboxUpgradePhaseRestore, err)
	}
	if err := h.runRestore(ctx, backupMeta, newSandbox); err != nil {
		return fail(model.EmployeeSandboxUpgradePhaseRestore, err)
	}

	// Phase 4: Restart new sandbox so the runtime opens the restored DB cleanly.
	if err := h.markPhase(ctx, upgrade, model.EmployeeSandboxUpgradePhaseRestartNew); err != nil {
		return fail(model.EmployeeSandboxUpgradePhaseRestartNew, err)
	}
	if err := h.orchestrator.RestartEmployeeSandbox(ctx, newSandbox); err != nil {
		return fail(model.EmployeeSandboxUpgradePhaseRestartNew, err)
	}
	// RestartEmployeeSandbox flips the row back to 'running'; re-park it as
	// 'upgrading' so it stays non-selectable until the sync below succeeds.
	if err := h.db.WithContext(ctx).Model(newSandbox).Update("status", string(sandbox.StatusUpgrading)).Error; err != nil {
		return fail(model.EmployeeSandboxUpgradePhaseRestartNew, fmt.Errorf("re-park new sandbox as upgrading: %w", err))
	}
	newSandbox.Status = string(sandbox.StatusUpgrading)

	// Phase 5: Sync current control-plane config to the restored runtime.
	if err := h.markPhase(ctx, upgrade, model.EmployeeSandboxUpgradePhaseSync); err != nil {
		return fail(model.EmployeeSandboxUpgradePhaseSync, err)
	}
	if err := h.syncEmployeeRuntime(ctx, agent, newSandbox); err != nil {
		return fail(model.EmployeeSandboxUpgradePhaseSync, err)
	}
	// The new runtime is restored, restarted and synced. Flip it to 'running'
	// now so the selector starts routing live traffic to it — and only now, after
	// the old session DB has been transferred, so no pre-restore turn is lost.
	activatedAt := time.Now()
	if err := h.db.WithContext(ctx).Model(newSandbox).Updates(map[string]any{
		"status":         string(sandbox.StatusRunning),
		"last_active_at": activatedAt,
	}).Error; err != nil {
		return fail(model.EmployeeSandboxUpgradePhaseSync, fmt.Errorf("activate new sandbox: %w", err))
	}
	newSandbox.Status = string(sandbox.StatusRunning)
	newSandbox.LastActiveAt = &activatedAt

	// Phase 6: Pause old sandbox only after the new one is restored and ready.
	if err := h.markPhase(ctx, upgrade, model.EmployeeSandboxUpgradePhasePausingOld); err != nil {
		return fail(model.EmployeeSandboxUpgradePhasePausingOld, err)
	}
	if err := h.orchestrator.StopSandbox(ctx, oldSandbox); err != nil {
		// Some providers (Railway) have no pause primitive. That is fine here:
		// the new sandbox is already serving traffic and Phase 7 schedules the old
		// sandbox's retirement (deletion), so we don't roll back over an
		// unsupported pause — only over a real stop failure. oldPaused stays false
		// so a later-phase rollback won't try to restart a sandbox we never stopped.
		if !errors.Is(err, sandbox.ErrUnsupported) {
			return fail(model.EmployeeSandboxUpgradePhasePausingOld, err)
		}
		log.InfoContext(ctx, "employee sandbox upgrade: provider cannot pause old sandbox; relying on retirement to delete it",
			"upgrade_id", upgrade.ID, "old_sandbox_id", oldSandbox.ID)
	} else {
		oldPaused = true
	}

	// Phase 7: Schedule old sandbox retirement.
	if err := h.markPhase(ctx, upgrade, model.EmployeeSandboxUpgradePhaseCleanupOld); err != nil {
		return fail(model.EmployeeSandboxUpgradePhaseCleanupOld, err)
	}
	if err := h.scheduleOldSandboxRetirement(ctx, upgrade, oldSandbox); err != nil {
		logging.Capture(ctx, fmt.Errorf("employee sandbox upgrade %s: schedule old sandbox retirement failed: %w", upgrade.ID, err))
	}

	// Phase 8: Mark succeeded.
	now := time.Now().UTC()
	if err := h.db.WithContext(ctx).Model(upgrade).Updates(map[string]any{
		"status":       model.EmployeeSandboxUpgradeStatusSucceeded,
		"phase":        model.EmployeeSandboxUpgradePhaseCompleted,
		"completed_at": now,
	}).Error; err != nil {
		return fail(model.EmployeeSandboxUpgradePhaseCleanupOld, fmt.Errorf("mark employee sandbox upgrade succeeded: %w", err))
	}
	upgrade.Status = model.EmployeeSandboxUpgradeStatusSucceeded
	upgrade.Phase = model.EmployeeSandboxUpgradePhaseCompleted
	upgrade.CompletedAt = &now
	recordEmployeeSandboxUpgradeSuccess(ctx, upgrade)
	log.InfoContext(ctx, "employee sandbox upgrade succeeded",
		"upgrade_id", upgrade.ID,
		"employee_id", upgrade.EmployeeID,
		"old_sandbox_id", upgrade.OldSandboxID,
		"new_sandbox_id", upgrade.NewSandboxID,
	)
	return nil
}
