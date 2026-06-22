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

type AgentSandboxRetireHandler struct {
	db           *gorm.DB
	orchestrator *sandbox.Orchestrator
}

func NewAgentSandboxRetireHandler(db *gorm.DB, orchestrator *sandbox.Orchestrator) *AgentSandboxRetireHandler {
	return &AgentSandboxRetireHandler{db: db, orchestrator: orchestrator}
}

func (h *AgentSandboxRetireHandler) Handle(ctx context.Context, task *asynq.Task) error {
	if h == nil || h.db == nil || h.orchestrator == nil {
		return fmt.Errorf("agent sandbox retire handler not configured")
	}
	var payload AgentSandboxRetirePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal agent sandbox retire payload: %w", err)
	}
	if payload.UpgradeID == uuid.Nil || payload.AgentID == uuid.Nil || payload.SandboxID == uuid.Nil {
		return fmt.Errorf("agent sandbox retire payload missing ids")
	}
	return h.retire(ctx, payload)
}

func (h *AgentSandboxRetireHandler) retire(ctx context.Context, payload AgentSandboxRetirePayload) error {
	var upgrade model.AgentSandboxUpgrade
	if err := h.db.WithContext(ctx).First(&upgrade, "id = ? AND agent_id = ?", payload.UpgradeID, payload.AgentID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return fmt.Errorf("load agent sandbox upgrade: %w", err)
	}
	if upgrade.Status != model.AgentSandboxUpgradeStatusSucceeded ||
		upgrade.Phase != model.AgentSandboxUpgradePhaseCompleted ||
		upgrade.OldSandboxID == nil ||
		*upgrade.OldSandboxID != payload.SandboxID {
		return nil
	}

	var sb model.Sandbox
	if err := h.db.WithContext(ctx).First(&sb, "id = ?", payload.SandboxID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return fmt.Errorf("load old agent sandbox: %w", err)
	}
	if sb.AgentID == nil || *sb.AgentID != payload.AgentID {
		return nil
	}
	if upgrade.NewSandboxID != nil && *upgrade.NewSandboxID == sb.ID {
		return nil
	}
	recordAgentSandboxRetire(ctx, &upgrade, &sb)
	logging.FromContext(ctx).InfoContext(ctx, "retiring old agent sandbox",
		"upgrade_id", upgrade.ID,
		"agent_id", payload.AgentID,
		"sandbox_id", sb.ID,
		"external_id", sb.ExternalID,
	)
	// Release the provider resource but KEEP the control-plane row: a hard
	// DeleteSandbox would cascade-delete the org's pre-upgrade history (FK ON DELETE
	// CASCADE) the upgrade must carry forward.
	if err := h.orchestrator.DeleteSandboxResource(ctx, &sb); err != nil {
		return fmt.Errorf("retire agent sandbox resource: %w", err)
	}
	return nil
}
