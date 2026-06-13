package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

// Archive handles DELETE /v1/agents/{id}.
// @Summary Archive an agent
// @Description Archives an agent when it is not the default Hivy agent and has no active sessions.
// @Tags agents
// @Produce json
// @Param id path string true "Agent ID"
// @Success 200 {object} agentMutationResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/agents/{id} [delete]
func (h *AgentHandler) Archive(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logging.FromContext(ctx)
	org, ok := middleware.OrgFromContext(ctx)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing org context"})
		return
	}
	agentID, ok := agentIDFromRequest(w, r)
	if !ok {
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
		log.ErrorContext(ctx, "load agent for archive", "error", err, "agent_id", agentID, "org_id", org.ID)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load agent"})
		return
	}
	if agent.IsDefault {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "default agent cannot be archived"})
		return
	}
	hasActiveSessions, err := h.agentHasActiveSessions(ctx, org.ID, agent.ID)
	if err != nil {
		log.ErrorContext(ctx, "check active sessions before agent archive", "error", err, "agent_id", agent.ID, "org_id", org.ID)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to check active sessions"})
		return
	}
	if hasActiveSessions {
		writeJSON(w, http.StatusConflict, errorResponse{Error: "agent has active sessions"})
		return
	}
	if err := h.db.WithContext(ctx).
		Model(&model.Agent{}).
		Where("id = ? AND org_id = ?", agent.ID, org.ID).
		Update("status", "archived").Error; err != nil {
		log.ErrorContext(ctx, "archive agent", "error", err, "agent_id", agent.ID, "org_id", org.ID)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to archive agent"})
		return
	}
	agent.Status = "archived"
	writeJSON(w, http.StatusOK, agentMutationResponse{Agent: h.agentListItem(ctx, org.ID, agent)})
}

func (h *AgentHandler) agentHasActiveSessions(ctx context.Context, orgID, agentID uuid.UUID) (bool, error) {
	var newCount int64
	if err := h.db.WithContext(ctx).
		Model(&model.Session{}).
		Where("org_id = ? AND agent_id = ? AND status = ?", orgID, agentID, "active").
		Count(&newCount).Error; err != nil {
		return false, err
	}
	return newCount > 0, nil
}
