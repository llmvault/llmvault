package handler

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

type rebootAgentSandboxResponse struct {
	Agent     agentResponse     `json:"agent"`
	SandboxID string            `json:"sandbox_id"`
	Sync      syncAgentResponse `json:"sync"`
}

// RebootSandbox handles POST /v1/agents/{id}/sandbox/reboot.
// @Summary Reboot an agent sandbox
// @Description Restarts the agent sandbox, pushes fresh runtime config, mints fresh proxy credentials, and verifies readiness.
// @Tags agents
// @Produce json
// @Param id path string true "Agent ID"
// @Success 200 {object} rebootAgentSandboxResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Failure 502 {object} errorResponse
// @Security BearerAuth
// @Router /v1/agents/{id}/sandbox/reboot [post]
func (h *AgentHandler) RebootSandbox(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logging.FromContext(ctx)

	if h == nil || h.db == nil || h.orchestrator == nil || h.compileDeps.EncKey == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "agent sandbox reboot not configured"})
		return
	}

	org, ok := middleware.OrgFromContext(ctx)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing org context"})
		return
	}

	agentID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid agent id"})
		return
	}

	var agent model.Agent
	if err := h.db.WithContext(ctx).
		Where("id = ? AND org_id = ? AND status <> ?", agentID, org.ID, "archived").
		First(&agent).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "agent not found"})
			return
		}
		log.ErrorContext(ctx, "load agent for sandbox reboot", "error", err, "agent_id", agentID, "org_id", org.ID)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load agent"})
		return
	}

	if upgrade, ok, err := activeAgentSandboxUpgrade(ctx, h.db, org.ID, agentID); err != nil {
		log.ErrorContext(ctx, "load active agent sandbox upgrade for reboot", "error", err, "agent_id", agentID)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load active upgrade"})
		return
	} else if ok {
		writeAgentUpgradeConflict(w, upgrade)
		return
	}

	sb, err := h.ensureAgentSandbox(ctx, &agent)
	if err != nil {
		log.ErrorContext(ctx, "ensure agent sandbox during reboot", "error", err, "agent_id", agentID)
		logging.Capture(ctx, fmt.Errorf("ensure agent sandbox during reboot: %w", err))
		writeJSON(w, http.StatusBadGateway, errorResponse{Error: "failed to provision agent sandbox"})
		return
	}

	if err := h.orchestrator.RestartAgentSandbox(ctx, sb); err != nil {
		log.ErrorContext(ctx, "restart agent sandbox", "error", err, "agent_id", agentID, "sandbox_id", sb.ID)
		logging.Capture(ctx, fmt.Errorf("restart agent sandbox: %w", err))
		writeJSON(w, http.StatusBadGateway, errorResponse{Error: "failed to restart agent sandbox"})
		return
	}

	syncResp, err := h.runAgentSync(ctx, &agent, sb)
	if err != nil {
		log.ErrorContext(ctx, "sync agent after sandbox reboot", "error", err, "agent_id", agentID, "sandbox_id", sb.ID)
		logging.Capture(ctx, fmt.Errorf("sync agent after sandbox reboot: %w", err))
		writeJSON(w, http.StatusBadGateway, errorResponse{Error: "agent sandbox restarted, but rejected sync"})
		return
	}

	log.InfoContext(ctx, "agent sandbox rebooted and synced", "agent_id", agent.ID, "sandbox_id", sb.ID)
	writeJSON(w, http.StatusOK, rebootAgentSandboxResponse{
		Agent:     toAgentResponse(agent),
		SandboxID: sb.ID.String(),
		Sync:      toSyncResponseDTO(syncResp),
	})
}
