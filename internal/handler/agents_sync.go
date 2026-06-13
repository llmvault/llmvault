package handler

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

type syncAgentResponse struct {
	Applied          int      `json:"applied"`
	Deleted          int      `json:"deleted"`
	ReposCloned      int      `json:"repos_cloned"`
	RestartTriggered bool     `json:"restart_triggered"`
	Errors           []string `json:"errors,omitempty"`
}

// @Summary Push compiled config to an agent sandbox
// @Description Compiles the agent config, provisions an agent sandbox if
// @Description needed, pushes it to the runtime, and verifies readiness.
// @Tags agents
// @Produce json
// @Param id path string true "Agent UUID"
// @Success 200 {object} syncAgentResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Failure 502 {object} errorResponse
// @Security BearerAuth
// @Router /v1/agents/{id}/sync [post]
func (h *AgentHandler) Sync(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logging.FromContext(ctx)

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

	var agent model.Agent
	if err := h.db.WithContext(ctx).Where("id = ? AND org_id = ?", agentID, org.ID).First(&agent).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
			return
		}
		log.ErrorContext(ctx, "load agent", "error", err, "agent_id", agentID, "org_id", org.ID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load agent"})
		return
	}

	if upgrade, ok, err := activeAgentSandboxUpgrade(ctx, h.db, org.ID, agentID); err != nil {
		log.ErrorContext(ctx, "load active agent sandbox upgrade", "error", err, "agent_id", agentID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load active upgrade"})
		return
	} else if ok {
		writeAgentUpgradeConflict(w, upgrade)
		return
	}

	sb, err := h.ensureAgentSandbox(ctx, &agent)
	if err != nil {
		log.ErrorContext(ctx, "provision agent sandbox during sync", "error", err, "agent_id", agentID)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to provision agent sandbox"})
		return
	}

	resp, err := h.runAgentSync(ctx, &agent, sb)
	if err != nil {
		log.ErrorContext(ctx, "sync agent config", "error", err,
			"agent_id", agentID, "sandbox_id", sb.ID)
		logging.Capture(ctx, err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "sandbox rejected sync"})
		return
	}
	writeJSON(w, http.StatusOK, toSyncResponseDTO(resp))
}
