package tasks

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

func (h *AgentSandboxUpgradeHandler) loadAndStart(ctx context.Context, payload AgentSandboxUpgradePayload) (*model.AgentSandboxUpgrade, *model.Agent, *model.Sandbox, error) {
	var upgrade model.AgentSandboxUpgrade
	err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND agent_id = ?", payload.UpgradeID, payload.AgentID).
			First(&upgrade).Error; err != nil {
			return err
		}
		switch upgrade.Status {
		case model.AgentSandboxUpgradeStatusSucceeded, model.AgentSandboxUpgradeStatusFailed:
			return nil
		case model.AgentSandboxUpgradeStatusQueued, model.AgentSandboxUpgradeStatusRunning:
		default:
			return fmt.Errorf("unsupported agent sandbox upgrade status %q", upgrade.Status)
		}
		return tx.Model(&upgrade).Updates(map[string]any{
			"status": model.AgentSandboxUpgradeStatusRunning,
			"phase":  model.AgentSandboxUpgradePhaseCreatingNew,
		}).Error
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, nil, nil
		}
		return nil, nil, nil, fmt.Errorf("load agent sandbox upgrade: %w", err)
	}
	if upgrade.Status == model.AgentSandboxUpgradeStatusSucceeded || upgrade.Status == model.AgentSandboxUpgradeStatusFailed {
		return nil, nil, nil, nil
	}
	upgrade.Status = model.AgentSandboxUpgradeStatusRunning
	upgrade.Phase = model.AgentSandboxUpgradePhaseCreatingNew

	var agent model.Agent
	if err := h.db.WithContext(ctx).
		Where("id = ? AND org_id = ? AND status <> ?", upgrade.AgentID, upgrade.OrgID, "archived").
		First(&agent).Error; err != nil {
		h.markFailed(ctx, &upgrade, model.AgentSandboxUpgradePhaseCreatingNew, "agent not found")
		return nil, nil, nil, fmt.Errorf("load agent: %w", err)
	}
	if agent.OrgID == nil {
		h.markFailed(ctx, &upgrade, model.AgentSandboxUpgradePhaseCreatingNew, "agent missing org")
		return nil, nil, nil, fmt.Errorf("agent missing org")
	}
	if agent.SandboxStrategy == agentSandboxStrategyPerSession {
		h.markFailed(ctx, &upgrade, model.AgentSandboxUpgradePhaseCreatingNew, "per-session agents do not use sandbox upgrades")
		return nil, nil, nil, nil
	}
	selector := agentRuntimeSelector(h.db, h.compileDeps)
	var oldSandbox *model.Sandbox
	var oldSandboxErr error
	if upgrade.OldSandboxID != nil {
		var recorded model.Sandbox
		oldSandboxErr = h.db.WithContext(ctx).
			Where("id = ? AND org_id = ? AND agent_id = ?", *upgrade.OldSandboxID, upgrade.OrgID, upgrade.AgentID).
			First(&recorded).Error
		oldSandbox = &recorded
	} else {
		oldSandbox, oldSandboxErr = selector.MainRuntime(ctx, upgrade.OrgID, upgrade.AgentID)
	}
	if oldSandboxErr != nil {
		h.markFailed(ctx, &upgrade, model.AgentSandboxUpgradePhaseCreatingNew, "current sandbox not found")
		return nil, nil, nil, fmt.Errorf("load current sandbox: %w", oldSandboxErr)
	}
	if upgrade.OldSandboxID == nil {
		if err := h.db.WithContext(ctx).Model(&upgrade).Update("old_sandbox_id", oldSandbox.ID).Error; err != nil {
			return nil, nil, nil, fmt.Errorf("record old sandbox: %w", err)
		}
		upgrade.OldSandboxID = &oldSandbox.ID
	}
	return &upgrade, &agent, oldSandbox, nil
}

func (h *AgentSandboxUpgradeHandler) syncAgentRuntime(ctx context.Context, agent *model.Agent, sb *model.Sandbox) error {
	apiKey, err := h.compileDeps.EncKey.DecryptString(sb.EncryptedRuntimeSecret)
	if err != nil {
		return fmt.Errorf("decrypt runtime secret: %w", err)
	}
	configUpdate, _, err := agentruntime.BuildAgentRuntimeConfigUpdate(ctx, h.compileDeps, agent, sb, apiKey)
	if err != nil {
		return fmt.Errorf("build agent runtime config: %w", err)
	}
	client := agentruntime.NewClient(sb.RuntimeURL, apiKey)
	if err := client.Healthz(ctx); err != nil {
		return fmt.Errorf("agent runtime healthz: %w", err)
	}
	if _, err := client.PutRuntimeConfig(ctx, configUpdate); err != nil {
		return fmt.Errorf("agent runtime put config: %w", err)
	}
	if err := client.Readyz(ctx); err != nil {
		return fmt.Errorf("agent runtime readyz: %w", err)
	}
	if agent.Status != "active" {
		if err := h.db.WithContext(ctx).Model(&model.Agent{}).
			Where("id = ? AND org_id = ?", agent.ID, *agent.OrgID).
			Update("status", "active").Error; err != nil {
			return fmt.Errorf("mark agent active: %w", err)
		}
		agent.Status = "active"
	}
	return nil
}

func (h *AgentSandboxUpgradeHandler) markPhase(ctx context.Context, upgrade *model.AgentSandboxUpgrade, phase string) error {
	if err := h.db.WithContext(ctx).Model(upgrade).Updates(map[string]any{
		"status": model.AgentSandboxUpgradeStatusRunning,
		"phase":  phase,
	}).Error; err != nil {
		return fmt.Errorf("mark agent sandbox upgrade %s: %w", phase, err)
	}
	upgrade.Status = model.AgentSandboxUpgradeStatusRunning
	upgrade.Phase = phase
	recordAgentSandboxUpgradePhase(ctx, upgrade, phase)
	return nil
}

func (h *AgentSandboxUpgradeHandler) markFailed(ctx context.Context, upgrade *model.AgentSandboxUpgrade, phase, message string) {
	now := time.Now().UTC()
	truncated := truncateUpgradeError(message)
	if err := h.db.WithContext(ctx).Model(upgrade).Updates(map[string]any{
		"status":        model.AgentSandboxUpgradeStatusFailed,
		"phase":         phase,
		"error_message": truncated,
		"completed_at":  now,
	}).Error; err != nil {
		logging.Capture(ctx, fmt.Errorf("agent sandbox upgrade: mark failed: %w", err))
	}
	upgrade.Status = model.AgentSandboxUpgradeStatusFailed
	upgrade.Phase = phase
	upgrade.ErrorMessage = &truncated
	upgrade.CompletedAt = &now
	recordAgentSandboxUpgradeFailure(ctx, upgrade, phase, truncated)
}

func truncateUpgradeError(msg string) string {
	msg = strings.TrimSpace(msg)
	if len(msg) > 2000 {
		return msg[:2000]
	}
	return msg
}
