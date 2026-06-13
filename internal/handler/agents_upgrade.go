package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/tasks"
)

type startAgentSandboxUpgradeRequest struct{}

type agentSandboxUpgradeResponse struct {
	UpgradeID    string     `json:"upgrade_id"`
	Status       string     `json:"status"`
	Phase        string     `json:"phase"`
	OldSandboxID *string    `json:"old_sandbox_id,omitempty"`
	NewSandboxID *string    `json:"new_sandbox_id,omitempty"`
	BackupKey    *string    `json:"backup_key,omitempty"`
	BackupSHA256 *string    `json:"backup_sha256,omitempty"`
	BackupBytes  int64      `json:"backup_bytes,omitempty"`
	ErrorMessage *string    `json:"error_message,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
}

// @Summary Start an agent sandbox upgrade
// @Description Queues a control-plane upgrade that snapshots the agent runtime SQLite database,
// @Description recreates the sandbox on the current agent image, restores the database,
// @Description syncs config, verifies readiness, pauses the old sandbox, and schedules cleanup.
// @Description If an upgrade is already queued or running for the agent, the active operation is returned.
// @Tags agents
// @Accept json
// @Produce json
// @Param id path string true "Agent agent ID"
// @Success 202 {object} agentSandboxUpgradeResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Failure 503 {object} errorResponse
// @Security BearerAuth
// @Router /v1/agents/{id}/sandbox/upgrade [post]
func (h *AgentHandler) StartSandboxUpgrade(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logging.FromContext(ctx)

	if h.enqueuer == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "agent sandbox upgrades not configured"})
		return
	}
	org, ok := middleware.OrgFromContext(ctx)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}
	agentID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent id"})
		return
	}
	var req startAgentSandboxUpgradeRequest
	if r.Body != nil && r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
	}

	if existing, ok, err := activeAgentSandboxUpgrade(ctx, h.db, org.ID, agentID); err != nil {
		log.ErrorContext(ctx, "load active agent sandbox upgrade", "error", err, "agent_id", agentID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load active upgrade"})
		return
	} else if ok {
		writeJSON(w, http.StatusAccepted, toAgentSandboxUpgradeResponse(existing))
		return
	}

	var agent model.Agent
	if err := h.db.WithContext(ctx).
		Where("id = ? AND org_id = ? AND status <> ?", agentID, org.ID, "archived").
		First(&agent).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
			return
		}
		log.ErrorContext(ctx, "load agent for sandbox upgrade", "error", err, "agent_id", agentID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load agent"})
		return
	}
	if err := h.deleteStaleAgentSandboxUpgradeTask(agentID); err != nil {
		log.ErrorContext(ctx, "delete stale agent sandbox upgrade task", "error", err, "agent_id", agentID)
		if strings.Contains(err.Error(), "active state") {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "agent sandbox upgrade task is already running"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to clear stale upgrade task"})
		return
	}

	oldSandbox, err := h.mainAgentRuntimeSelector().MainRuntime(ctx, org.ID, agentID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "sandbox not found for agent"})
			return
		}
		log.ErrorContext(ctx, "load agent sandbox for upgrade", "error", err, "agent_id", agentID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load agent sandbox"})
		return
	}

	upgrade := model.AgentSandboxUpgrade{
		OrgID:        org.ID,
		AgentID:      agent.ID,
		OldSandboxID: &oldSandbox.ID,
		Status:       model.AgentSandboxUpgradeStatusQueued,
		Phase:        model.AgentSandboxUpgradePhaseQueued,
	}
	if err := h.db.WithContext(ctx).Create(&upgrade).Error; err != nil {
		log.ErrorContext(ctx, "create agent sandbox upgrade", "error", err, "agent_id", agentID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create upgrade"})
		return
	}

	task, opts, err := tasks.NewAgentSandboxUpgradeTask(upgrade.ID, agent.ID)
	if err != nil {
		h.markUpgradeFailed(ctx, &upgrade, model.AgentSandboxUpgradePhaseQueued, err.Error())
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to build upgrade task"})
		return
	}
	if _, err := h.enqueuer.EnqueueContext(ctx, task, opts...); err != nil {
		h.markUpgradeFailed(ctx, &upgrade, model.AgentSandboxUpgradePhaseQueued, err.Error())
		if errors.Is(err, asynq.ErrTaskIDConflict) || errors.Is(err, asynq.ErrDuplicateTask) {
			if existing, ok, loadErr := activeAgentSandboxUpgrade(ctx, h.db, org.ID, agentID); loadErr == nil && ok {
				writeJSON(w, http.StatusAccepted, toAgentSandboxUpgradeResponse(existing))
				return
			}
		}
		log.ErrorContext(ctx, "enqueue agent sandbox upgrade", "error", err, "upgrade_id", upgrade.ID, "agent_id", agentID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to enqueue upgrade"})
		return
	}

	writeJSON(w, http.StatusAccepted, toAgentSandboxUpgradeResponse(&upgrade))
}

// @Summary Get an agent sandbox upgrade
// @Description Returns the current status and phase for a sandbox upgrade operation.
// @Tags agents
// @Produce json
// @Param id path string true "Agent agent ID"
// @Param upgradeID path string true "Upgrade operation ID"
// @Success 200 {object} agentSandboxUpgradeResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/agents/{id}/sandbox/upgrades/{upgradeID} [get]
func (h *AgentHandler) GetSandboxUpgrade(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	org, ok := middleware.OrgFromContext(ctx)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}
	agentID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent id"})
		return
	}
	upgradeID, err := uuid.Parse(chi.URLParam(r, "upgradeID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid upgrade id"})
		return
	}
	var upgrade model.AgentSandboxUpgrade
	if err := h.db.WithContext(ctx).
		Where("id = ? AND org_id = ? AND agent_id = ?", upgradeID, org.ID, agentID).
		First(&upgrade).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "upgrade not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load upgrade"})
		return
	}
	writeJSON(w, http.StatusOK, toAgentSandboxUpgradeResponse(&upgrade))
}

func (h *AgentHandler) markUpgradeFailed(ctx context.Context, upgrade *model.AgentSandboxUpgrade, phase, msg string) {
	now := time.Now()
	_ = h.db.WithContext(ctx).Model(upgrade).Updates(map[string]any{
		"status":        model.AgentSandboxUpgradeStatusFailed,
		"phase":         phase,
		"error_message": msg,
		"completed_at":  now,
	}).Error
	upgrade.Status = model.AgentSandboxUpgradeStatusFailed
	upgrade.Phase = phase
	upgrade.ErrorMessage = &msg
	upgrade.CompletedAt = &now
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

func writeAgentUpgradeConflict(w http.ResponseWriter, upgrade *model.AgentSandboxUpgrade) {
	writeJSON(w, http.StatusConflict, map[string]string{
		"error":      "agent sandbox upgrade in progress",
		"upgrade_id": upgrade.ID.String(),
		"status":     upgrade.Status,
		"phase":      upgrade.Phase,
	})
}

func toAgentSandboxUpgradeResponse(upgrade *model.AgentSandboxUpgrade) agentSandboxUpgradeResponse {
	resp := agentSandboxUpgradeResponse{
		UpgradeID:    upgrade.ID.String(),
		Status:       upgrade.Status,
		Phase:        upgrade.Phase,
		ErrorMessage: upgrade.ErrorMessage,
		BackupKey:    upgrade.BackupKey,
		BackupSHA256: upgrade.BackupSHA256,
		BackupBytes:  upgrade.BackupBytes,
		CreatedAt:    upgrade.CreatedAt,
		UpdatedAt:    upgrade.UpdatedAt,
		CompletedAt:  upgrade.CompletedAt,
	}
	if upgrade.OldSandboxID != nil {
		id := upgrade.OldSandboxID.String()
		resp.OldSandboxID = &id
	}
	if upgrade.NewSandboxID != nil {
		id := upgrade.NewSandboxID.String()
		resp.NewSandboxID = &id
	}
	return resp
}
