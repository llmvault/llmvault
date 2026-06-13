package tasks

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

const agentSandboxAutoUpgradeTimeout = 10 * time.Minute

type AgentSandboxAutoUpgradePayload struct {
	RuntimeImage string `json:"runtime_image"`
	Limit        int    `json:"limit,omitempty"`
}

type AgentSandboxAutoUpgradeHandler struct {
	db          *gorm.DB
	compileDeps agentruntime.CompileDeps
	enqueuer    enqueue.TaskEnqueuer
}

type outdatedAgentSandbox struct {
	OrgID     uuid.UUID
	AgentID   uuid.UUID
	SandboxID uuid.UUID
}

func NewAgentSandboxAutoUpgradeTask(payload AgentSandboxAutoUpgradePayload) (*asynq.Task, []asynq.Option, error) {
	payload.RuntimeImage = strings.TrimSpace(payload.RuntimeImage)
	if payload.RuntimeImage == "" {
		return nil, nil, fmt.Errorf("agent sandbox auto-upgrade runtime image is required")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal agent sandbox auto-upgrade payload: %w", err)
	}
	opts := []asynq.Option{
		asynq.Queue(QueueDefault),
		asynq.MaxRetry(3),
		asynq.Timeout(agentSandboxAutoUpgradeTimeout),
		asynq.Unique(10 * time.Minute),
		asynq.TaskID("agent-sandbox-auto-upgrade:" + shortHash(payload.RuntimeImage)),
	}
	return asynq.NewTask(TypeAgentSandboxAutoUpgrade, body), opts, nil
}

func EnqueueAgentSandboxAutoUpgrade(ctx context.Context, enqueuer enqueue.TaskEnqueuer, payload AgentSandboxAutoUpgradePayload) error {
	if enqueuer == nil {
		return nil
	}
	task, opts, err := NewAgentSandboxAutoUpgradeTask(payload)
	if err != nil {
		return err
	}
	if _, err := enqueuer.EnqueueContext(ctx, task, opts...); err != nil {
		if errors.Is(err, asynq.ErrDuplicateTask) || errors.Is(err, asynq.ErrTaskIDConflict) {
			return nil
		}
		return err
	}
	return nil
}

func NewAgentSandboxAutoUpgradeHandler(db *gorm.DB, compileDeps agentruntime.CompileDeps, enqueuer enqueue.TaskEnqueuer) *AgentSandboxAutoUpgradeHandler {
	return &AgentSandboxAutoUpgradeHandler{db: db, compileDeps: compileDeps, enqueuer: enqueuer}
}

func (h *AgentSandboxAutoUpgradeHandler) Handle(ctx context.Context, task *asynq.Task) error {
	if h == nil || h.db == nil || h.enqueuer == nil {
		return fmt.Errorf("agent sandbox auto-upgrade handler not configured")
	}
	if h.compileDeps.Cfg == nil {
		return fmt.Errorf("agent sandbox auto-upgrade config not configured")
	}
	var payload AgentSandboxAutoUpgradePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal agent sandbox auto-upgrade payload: %w", err)
	}
	payload.RuntimeImage = strings.TrimSpace(payload.RuntimeImage)
	if payload.RuntimeImage == "" {
		return fmt.Errorf("agent sandbox auto-upgrade runtime image is required")
	}
	rows, err := h.loadOutdatedAgentSandboxes(ctx, payload.RuntimeImage, payload.Limit)
	if err != nil {
		return err
	}
	var enqueued, skipped int
	for _, row := range rows {
		ok, err := h.enqueueUpgrade(ctx, row)
		if err != nil {
			logging.FromContext(ctx).ErrorContext(ctx, "agent sandbox auto-upgrade enqueue failed",
				"error", err, "agent_id", row.AgentID, "sandbox_id", row.SandboxID)
			continue
		}
		if ok {
			enqueued++
		} else {
			skipped++
		}
	}
	logging.FromContext(ctx).InfoContext(ctx, "agent sandbox auto-upgrade sweep complete",
		"runtime_image", payload.RuntimeImage, "candidates", len(rows), "enqueued", enqueued, "skipped", skipped)
	return nil
}

func (h *AgentSandboxAutoUpgradeHandler) loadOutdatedAgentSandboxes(ctx context.Context, runtimeImage string, limit int) ([]outdatedAgentSandbox, error) {
	if limit <= 0 {
		limit = 1000
	}
	args := []any{}
	query := `
WITH latest AS (
	SELECT DISTINCT ON (s.agent_id)
		s.org_id,
		s.agent_id,
		s.id AS sandbox_id,
		s.snapshot_id
	FROM sandboxes s
	JOIN agents e ON e.id = s.agent_id AND e.org_id = s.org_id
	WHERE s.status = 'running'
		AND s.org_id IS NOT NULL
		AND s.agent_id IS NOT NULL
		AND e.status <> 'archived'
`
	query += `
	ORDER BY s.agent_id, s.created_at DESC
)
SELECT org_id, agent_id, sandbox_id
FROM latest
WHERE snapshot_id IS NULL OR snapshot_id <> ?
LIMIT ?
`
	args = append(args, runtimeImage, limit)
	var rows []outdatedAgentSandbox
	if err := h.db.WithContext(ctx).Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("load outdated agent sandboxes: %w", err)
	}
	return rows, nil
}

func (h *AgentSandboxAutoUpgradeHandler) enqueueUpgrade(ctx context.Context, row outdatedAgentSandbox) (bool, error) {
	if active, ok, err := activeAgentSandboxUpgrade(ctx, h.db, row.OrgID, row.AgentID); err != nil {
		return false, err
	} else if ok && active != nil {
		return false, nil
	}
	if cleaner, ok := h.enqueuer.(enqueue.TaskCleaner); ok {
		if err := cleaner.DeleteTask(QueueBulk, AgentSandboxUpgradeTaskID(row.AgentID)); err != nil && !strings.Contains(err.Error(), "not found") {
			if strings.Contains(err.Error(), "active state") {
				return false, nil
			}
			return false, err
		}
	}
	upgrade := model.AgentSandboxUpgrade{
		OrgID:        row.OrgID,
		AgentID:      row.AgentID,
		OldSandboxID: &row.SandboxID,
		Status:       model.AgentSandboxUpgradeStatusQueued,
		Phase:        model.AgentSandboxUpgradePhaseQueued,
	}
	if err := h.db.WithContext(ctx).Create(&upgrade).Error; err != nil {
		return false, fmt.Errorf("create agent sandbox upgrade: %w", err)
	}
	task, opts, err := NewAgentSandboxUpgradeTask(upgrade.ID, row.AgentID)
	if err != nil {
		h.markFailed(ctx, &upgrade, model.AgentSandboxUpgradePhaseQueued, err.Error())
		return false, err
	}
	if _, err := h.enqueuer.EnqueueContext(ctx, task, opts...); err != nil {
		h.markFailed(ctx, &upgrade, model.AgentSandboxUpgradePhaseQueued, err.Error())
		if errors.Is(err, asynq.ErrTaskIDConflict) || errors.Is(err, asynq.ErrDuplicateTask) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func activeAgentSandboxUpgrade(ctx context.Context, db *gorm.DB, orgID, agentID uuid.UUID) (*model.AgentSandboxUpgrade, bool, error) {
	var upgrade model.AgentSandboxUpgrade
	err := db.WithContext(ctx).
		Where("org_id = ? AND agent_id = ? AND status IN ?", orgID, agentID, []string{
			model.AgentSandboxUpgradeStatusQueued,
			model.AgentSandboxUpgradeStatusRunning,
		}).
		Order("created_at DESC").
		First(&upgrade).Error
	if err == nil {
		return &upgrade, true, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	return nil, false, err
}

func (h *AgentSandboxAutoUpgradeHandler) markFailed(ctx context.Context, upgrade *model.AgentSandboxUpgrade, phase, message string) {
	now := time.Now().UTC()
	truncated := truncateUpgradeError(message)
	_ = h.db.WithContext(ctx).Model(upgrade).Updates(map[string]any{
		"status":        model.AgentSandboxUpgradeStatusFailed,
		"phase":         phase,
		"error_message": truncated,
		"completed_at":  now,
	}).Error
}

func shortHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])[:16]
}
