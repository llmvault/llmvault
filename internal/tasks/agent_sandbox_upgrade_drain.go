package tasks

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sandbox"
)

func (h *AgentSandboxUpgradeHandler) prepareOldSandboxForReplacement(ctx context.Context, agent *model.Agent, oldSandbox *model.Sandbox) (bool, error) {
	if agent == nil || agent.OrgID == nil {
		return false, fmt.Errorf("agent with org is required")
	}
	if oldSandbox == nil || oldSandbox.ID == uuid.Nil {
		return false, fmt.Errorf("old sandbox is required")
	}
	if err := h.db.WithContext(ctx).Model(oldSandbox).Updates(map[string]any{
		"status":        string(sandbox.StatusDraining),
		"error_message": nil,
	}).Error; err != nil {
		return false, fmt.Errorf("mark old sandbox draining: %w", err)
	}
	oldSandbox.Status = string(sandbox.StatusDraining)
	oldSandbox.ErrorMessage = nil

	activeSessions, err := h.countNonIdleSessionsForAgent(ctx, *agent.OrgID, agent.ID)
	if err != nil {
		return false, err
	}
	return activeSessions > 0, nil
}

func (h *AgentSandboxUpgradeHandler) waitForOldSandboxRuntimeDrain(ctx context.Context, oldSandbox *model.Sandbox) error {
	if oldSandbox == nil || oldSandbox.ID == uuid.Nil {
		return fmt.Errorf("old sandbox is required")
	}
	client, err := h.runtimeClientForSandbox(oldSandbox)
	if err != nil {
		return err
	}
	if _, err := client.StartDrain(ctx); err != nil {
		return fmt.Errorf("send runtime drain signal: %w", err)
	}

	pollCtx, cancel := context.WithTimeout(ctx, agentSandboxDrainTimeout)
	defer cancel()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		status, err := client.DrainStatus(pollCtx)
		if err != nil {
			return fmt.Errorf("poll runtime drain status: %w", err)
		}
		if status.Complete {
			return nil
		}
		select {
		case <-pollCtx.Done():
			return fmt.Errorf(
				"runtime drain timed out after %s (active_turns=%d pending_accepted_messages=%d pending_outbox_events=%d)",
				agentSandboxDrainTimeout,
				status.ActiveTurns,
				status.PendingAcceptedMessages,
				status.PendingOutboxEvents,
			)
		case <-ticker.C:
		}
	}
}

func (h *AgentSandboxUpgradeHandler) countNonIdleSessionsForAgent(ctx context.Context, orgID, agentID uuid.UUID) (int64, error) {
	var count int64
	err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("LOCK TABLE sessions IN SHARE MODE").Error; err != nil {
			return fmt.Errorf("lock sessions before drain decision: %w", err)
		}
		return tx.Model(&model.Session{}).
			Where("org_id = ? AND agent_id = ? AND agent_turn_status <> ?", orgID, agentID, model.SessionAgentTurnIdle).
			Count(&count).Error
	})
	if err != nil {
		return 0, fmt.Errorf("count active sessions before drain decision: %w", err)
	}
	return count, nil
}

func (h *AgentSandboxUpgradeHandler) restoreOldSandboxAfterDrainFailure(ctx context.Context, oldSandbox *model.Sandbox) error {
	if oldSandbox == nil || oldSandbox.ID == uuid.Nil {
		return nil
	}
	client, err := h.runtimeClientForSandbox(oldSandbox)
	var cancelErr error
	if err == nil {
		if _, cancelErr = client.CancelDrain(ctx); cancelErr == nil {
			return h.markOldSandboxRunning(ctx, oldSandbox)
		}
	} else {
		cancelErr = err
	}
	var restartErr error
	if h.orchestrator != nil {
		if restartErr = h.orchestrator.RestartAgentSandbox(ctx, oldSandbox); restartErr == nil {
			return nil
		}
	}
	return fmt.Errorf("cancel runtime drain failed: %v; restart old sandbox failed: %v", cancelErr, restartErr)
}

func (h *AgentSandboxUpgradeHandler) markOldSandboxRunning(ctx context.Context, oldSandbox *model.Sandbox) error {
	if err := h.db.WithContext(ctx).Model(oldSandbox).Updates(map[string]any{
		"status":        string(sandbox.StatusRunning),
		"error_message": nil,
	}).Error; err != nil {
		return fmt.Errorf("mark old sandbox running: %w", err)
	}
	oldSandbox.Status = string(sandbox.StatusRunning)
	oldSandbox.ErrorMessage = nil
	return nil
}

func (h *AgentSandboxUpgradeHandler) stopDrainedOldSandbox(ctx context.Context, oldSandbox *model.Sandbox) error {
	if oldSandbox == nil || oldSandbox.ID == uuid.Nil {
		return nil
	}
	if err := h.orchestrator.StopSandbox(ctx, oldSandbox); err != nil {
		if errors.Is(err, sandbox.ErrUnsupported) {
			return h.orchestrator.DeleteSandboxResource(ctx, oldSandbox)
		}
		return err
	}
	return nil
}

func (h *AgentSandboxUpgradeHandler) runtimeClientForSandbox(sb *model.Sandbox) (*agentruntime.Client, error) {
	if sb == nil || strings.TrimSpace(sb.RuntimeURL) == "" {
		return nil, fmt.Errorf("sandbox runtime URL is missing")
	}
	if h.compileDeps.EncKey == nil {
		return nil, fmt.Errorf("runtime encryption key is required")
	}
	apiKey, err := h.compileDeps.EncKey.DecryptString(sb.EncryptedRuntimeSecret)
	if err != nil {
		return nil, fmt.Errorf("decrypt runtime secret: %w", err)
	}
	return agentruntime.NewClientWithTimeout(sb.RuntimeURL, apiKey, 30*time.Second), nil
}

func (h *AgentSandboxUpgradeHandler) enqueuePendingSessionDeliveries(ctx context.Context, agentID uuid.UUID) error {
	if h.enqueuer == nil {
		return nil
	}
	if err := h.db.WithContext(ctx).Exec(`
UPDATE session_message_queue q
SET status = 'pending',
    leased_by = '',
    leased_until = NULL
FROM sessions s
WHERE q.session_id = s.id
  AND s.agent_id = ?
  AND q.status = 'processing'`, agentID).Error; err != nil {
		return fmt.Errorf("reset processing session messages: %w", err)
	}
	var sessionIDs []uuid.UUID
	if err := h.db.WithContext(ctx).Raw(`
SELECT DISTINCT q.session_id
FROM session_message_queue q
JOIN sessions s ON s.id = q.session_id
WHERE s.agent_id = ?
  AND q.status = 'pending'`, agentID).Scan(&sessionIDs).Error; err != nil {
		return fmt.Errorf("load pending session deliveries: %w", err)
	}
	for _, sessionID := range sessionIDs {
		if err := EnqueueSessionMessageDeliver(ctx, h.enqueuer, sessionID); err != nil {
			return fmt.Errorf("enqueue pending session delivery %s: %w", sessionID, err)
		}
	}
	return nil
}
