package tasks

import (
	"context"
	"fmt"
	"time"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

func (h *AgentSandboxUpgradeHandler) runInPlace(
	ctx context.Context,
	upgrade *model.AgentSandboxUpgrade,
	agent *model.Agent,
	oldSandbox *model.Sandbox,
	fail func(string, error) error,
) error {
	if err := h.recordInPlaceSandbox(ctx, upgrade, oldSandbox); err != nil {
		return fail(model.AgentSandboxUpgradePhaseCreatingNew, err)
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
		logging.FromContext(ctx).InfoContext(ctx, "agent sandbox upgrade skipping runtime drain because all sessions are idle",
			"upgrade_id", upgrade.ID,
			"agent_id", upgrade.AgentID,
			"old_sandbox_id", oldSandbox.ID,
		)
	}

	if err := h.markPhase(ctx, upgrade, model.AgentSandboxUpgradePhaseCreatingNew); err != nil {
		return fail(model.AgentSandboxUpgradePhaseCreatingNew, err)
	}
	secrets, err := agentruntime.PrepareStartup(ctx, h.compileDeps, agent)
	if err != nil {
		return fail(model.AgentSandboxUpgradePhaseCreatingNew, err)
	}
	upgradedSandbox, err := h.orchestrator.UpgradeAgentSandboxInPlace(ctx, agent, oldSandbox, secrets)
	if err != nil {
		return fail(model.AgentSandboxUpgradePhaseCreatingNew, err)
	}
	if err := h.markPhase(ctx, upgrade, model.AgentSandboxUpgradePhaseSync); err != nil {
		return fail(model.AgentSandboxUpgradePhaseSync, err)
	}
	if err := h.syncAgentRuntime(ctx, agent, upgradedSandbox); err != nil {
		return fail(model.AgentSandboxUpgradePhaseSync, err)
	}
	if err := h.enqueuePendingSessionDeliveries(ctx, agent.ID); err != nil {
		logging.Capture(ctx, fmt.Errorf("agent sandbox upgrade %s: enqueue pending session deliveries failed: %w", upgrade.ID, err))
	}
	return h.markInPlaceUpgradeSucceeded(ctx, upgrade)
}

func (h *AgentSandboxUpgradeHandler) recordInPlaceSandbox(ctx context.Context, upgrade *model.AgentSandboxUpgrade, sb *model.Sandbox) error {
	if sb == nil {
		return fmt.Errorf("sandbox is required")
	}
	if err := h.db.WithContext(ctx).Model(upgrade).Update("new_sandbox_id", sb.ID).Error; err != nil {
		return fmt.Errorf("record in-place sandbox: %w", err)
	}
	upgrade.NewSandboxID = &sb.ID
	recordAgentSandboxUpgradeNewSandbox(ctx, upgrade, sb)
	return nil
}

func (h *AgentSandboxUpgradeHandler) markInPlaceUpgradeSucceeded(ctx context.Context, upgrade *model.AgentSandboxUpgrade) error {
	now := time.Now().UTC()
	if err := h.db.WithContext(ctx).Model(upgrade).Updates(map[string]any{
		"status":       model.AgentSandboxUpgradeStatusSucceeded,
		"phase":        model.AgentSandboxUpgradePhaseCompleted,
		"completed_at": now,
	}).Error; err != nil {
		return fmt.Errorf("mark agent sandbox upgrade succeeded: %w", err)
	}
	upgrade.Status = model.AgentSandboxUpgradeStatusSucceeded
	upgrade.Phase = model.AgentSandboxUpgradePhaseCompleted
	upgrade.CompletedAt = &now
	recordAgentSandboxUpgradeSuccess(ctx, upgrade)
	logging.FromContext(ctx).InfoContext(ctx, "agent sandbox upgrade succeeded",
		"upgrade_id", upgrade.ID,
		"agent_id", upgrade.AgentID,
		"old_sandbox_id", upgrade.OldSandboxID,
		"new_sandbox_id", upgrade.NewSandboxID,
		"upgrade_mode", "in_place",
	)
	return nil
}
