package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sandbox"
)

const agentSandboxDrainTimeout = 10 * time.Minute

type AgentSandboxUpgradeHandler struct {
	db           *gorm.DB
	orchestrator *sandbox.Orchestrator
	compileDeps  agentruntime.CompileDeps
	enqueuer     enqueue.TaskEnqueuer
}

func NewAgentSandboxUpgradeHandler(
	db *gorm.DB,
	orchestrator *sandbox.Orchestrator,
	compileDeps agentruntime.CompileDeps,
	enqueuer enqueue.TaskEnqueuer,
) *AgentSandboxUpgradeHandler {
	return &AgentSandboxUpgradeHandler{
		db:           db,
		orchestrator: orchestrator,
		compileDeps:  compileDeps,
		enqueuer:     enqueuer,
	}
}

func (h *AgentSandboxUpgradeHandler) Handle(ctx context.Context, task *asynq.Task) error {
	if h == nil || h.db == nil || h.orchestrator == nil || h.compileDeps.EncKey == nil || h.enqueuer == nil {
		return fmt.Errorf("agent sandbox upgrade handler not configured")
	}
	var payload AgentSandboxUpgradePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal agent sandbox upgrade payload: %w", err)
	}
	if payload.UpgradeID == uuid.Nil || payload.AgentID == uuid.Nil {
		return fmt.Errorf("agent sandbox upgrade payload missing ids")
	}
	return h.run(ctx, payload)
}

func (h *AgentSandboxUpgradeHandler) run(ctx context.Context, payload AgentSandboxUpgradePayload) error {
	log := logging.FromContext(ctx)

	upgrade, agent, oldSandbox, err := h.loadAndStart(ctx, payload)
	if err != nil {
		return err
	}
	if upgrade == nil {
		return nil
	}
	annotateAgentSandboxUpgradeSentry(ctx, upgrade, agent, oldSandbox)

	var newSandbox *model.Sandbox
	fail := func(phase string, cause error) error {
		msg := cause.Error()
		// Rollback must run on WithoutCancel: a cancelled task ctx (asynq deadline, worker drain)
		// would no-op the provider delete and DB writes, stranding both sandboxes.
		rollbackCtx := context.WithoutCancel(ctx)
		recordAgentSandboxUpgradeFailure(rollbackCtx, upgrade, phase, msg)
		log.ErrorContext(ctx, "agent sandbox upgrade failed",
			"upgrade_id", upgrade.ID,
			"agent_id", upgrade.AgentID,
			"phase", phase,
			"error", msg,
		)
		if newSandbox != nil && newSandbox.ID != uuid.Nil {
			if err := h.orchestrator.DeleteSandbox(rollbackCtx, newSandbox); err != nil {
				msg += "; failed to delete new sandbox during rollback: " + err.Error()
			}
		}
		if oldSandbox != nil && oldSandbox.ID != uuid.Nil {
			if err := h.restoreOldSandboxAfterDrainFailure(rollbackCtx, oldSandbox); err != nil {
				msg += "; failed to restore old sandbox during rollback: " + err.Error()
			}
		}
		h.markFailed(rollbackCtx, upgrade, phase, msg)
		return cause
	}

	if err := h.markPhase(ctx, upgrade, model.AgentSandboxUpgradePhaseCreatingNew); err != nil {
		return err
	}
	secrets, err := agentruntime.PrepareStartup(ctx, h.compileDeps, agent)
	if err != nil {
		return fail(model.AgentSandboxUpgradePhaseCreatingNew, err)
	}
	newSandbox, err = h.orchestrator.CreateAgentSandbox(ctx, agent, secrets)
	if err != nil {
		return fail(model.AgentSandboxUpgradePhaseCreatingNew, err)
	}
	// CreateAgentSandbox marks it 'running', and the selector orders by
	// created_at DESC, so it would be picked for live traffic before sync.
	// Park it 'upgrading' (non-selectable) so turns stay on the old sandbox.
	if err := h.db.WithContext(ctx).Model(newSandbox).Update("status", string(sandbox.StatusUpgrading)).Error; err != nil {
		return fail(model.AgentSandboxUpgradePhaseCreatingNew, fmt.Errorf("park new sandbox as upgrading: %w", err))
	}
	newSandbox.Status = string(sandbox.StatusUpgrading)
	if err := h.db.WithContext(ctx).Model(upgrade).Update("new_sandbox_id", newSandbox.ID).Error; err != nil {
		return fail(model.AgentSandboxUpgradePhaseCreatingNew, err)
	}
	upgrade.NewSandboxID = &newSandbox.ID
	recordAgentSandboxUpgradeNewSandbox(ctx, upgrade, newSandbox)

	if err := h.markPhase(ctx, upgrade, model.AgentSandboxUpgradePhaseSync); err != nil {
		return fail(model.AgentSandboxUpgradePhaseSync, err)
	}
	if err := h.syncAgentRuntime(ctx, agent, newSandbox); err != nil {
		return fail(model.AgentSandboxUpgradePhaseSync, err)
	}

	needsRuntimeDrain, err := h.prepareOldSandboxForReplacement(ctx, agent, oldSandbox)
	if err != nil {
		return fail(model.AgentSandboxUpgradePhaseDrainingOld, err)
	}
	if needsRuntimeDrain {
		if err := h.markPhase(ctx, upgrade, model.AgentSandboxUpgradePhaseDrainingOld); err != nil {
			return fail(model.AgentSandboxUpgradePhaseDrainingOld, err)
		}
		if err := h.waitForOldSandboxRuntimeDrain(ctx, oldSandbox); err != nil {
			return fail(model.AgentSandboxUpgradePhaseDrainingOld, err)
		}
	} else {
		log.InfoContext(ctx, "agent sandbox upgrade skipping runtime drain because all sessions are idle",
			"upgrade_id", upgrade.ID,
			"agent_id", upgrade.AgentID,
			"old_sandbox_id", oldSandbox.ID,
		)
	}

	// The replacement is synced and the old runtime has drained all accepted
	// turns/webhooks. Only now flip to running so new traffic selects it.
	activatedAt := time.Now()
	if err := h.db.WithContext(ctx).Model(newSandbox).Updates(map[string]any{
		"status":         string(sandbox.StatusRunning),
		"last_active_at": activatedAt,
	}).Error; err != nil {
		return fail(model.AgentSandboxUpgradePhaseDrainingOld, fmt.Errorf("activate new sandbox: %w", err))
	}
	newSandbox.Status = string(sandbox.StatusRunning)
	newSandbox.LastActiveAt = &activatedAt
	if err := h.enqueuePendingSessionDeliveries(ctx, agent.ID); err != nil {
		logging.Capture(ctx, fmt.Errorf("agent sandbox upgrade %s: enqueue pending session deliveries failed: %w", upgrade.ID, err))
	}

	if err := h.markPhase(ctx, upgrade, model.AgentSandboxUpgradePhaseCleanupOld); err != nil {
		return fail(model.AgentSandboxUpgradePhaseCleanupOld, err)
	}
	if err := h.stopDrainedOldSandbox(ctx, oldSandbox); err != nil {
		logging.Capture(ctx, fmt.Errorf("agent sandbox upgrade %s: stop drained old sandbox failed: %w", upgrade.ID, err))
	}
	if err := h.scheduleOldSandboxRetirement(ctx, upgrade, oldSandbox); err != nil {
		logging.Capture(ctx, fmt.Errorf("agent sandbox upgrade %s: schedule old sandbox retirement failed: %w", upgrade.ID, err))
	}

	now := time.Now().UTC()
	if err := h.db.WithContext(ctx).Model(upgrade).Updates(map[string]any{
		"status":       model.AgentSandboxUpgradeStatusSucceeded,
		"phase":        model.AgentSandboxUpgradePhaseCompleted,
		"completed_at": now,
	}).Error; err != nil {
		return fail(model.AgentSandboxUpgradePhaseCleanupOld, fmt.Errorf("mark agent sandbox upgrade succeeded: %w", err))
	}
	upgrade.Status = model.AgentSandboxUpgradeStatusSucceeded
	upgrade.Phase = model.AgentSandboxUpgradePhaseCompleted
	upgrade.CompletedAt = &now
	recordAgentSandboxUpgradeSuccess(ctx, upgrade)
	log.InfoContext(ctx, "agent sandbox upgrade succeeded",
		"upgrade_id", upgrade.ID,
		"agent_id", upgrade.AgentID,
		"old_sandbox_id", upgrade.OldSandboxID,
		"new_sandbox_id", upgrade.NewSandboxID,
	)
	return nil
}
