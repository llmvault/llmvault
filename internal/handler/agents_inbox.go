package handler

import (
	"errors"
	"net/http"
	"strings"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/access"
	"github.com/usehivy/hivy/internal/agentemail"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

type agentInboxResponse struct {
	Available    bool   `json:"available"`
	Email        string `json:"email,omitempty"`
	MessageCount int64  `json:"message_count"`
}

// GetInbox handles GET /v1/agents/{id}/inbox.
// @Summary Get an agent inbox
// @Description Returns whether the agent has an inbox, its address when available, and the total number of received messages.
// @Tags agents
// @Produce json
// @Param id path string true "Agent ID"
// @Success 200 {object} agentInboxResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 503 {object} errorResponse
// @Security BearerAuth
// @Router /v1/agents/{id}/inbox [get]
func (h *AgentHandler) GetInbox(w http.ResponseWriter, r *http.Request) {
	agent, ok := h.authorizeAgentInbox(w, r)
	if !ok {
		return
	}
	inbox, err := agentemail.GetInbox(r.Context(), h.db, *agent.OrgID, agent.ID, h.agentInboxDomain())
	if err != nil {
		writeAgentInboxError(r, w, err)
		return
	}
	writeJSON(w, http.StatusOK, toAgentInboxResponse(inbox))
}

// ProvisionInbox handles POST /v1/agents/{id}/inbox.
// @Summary Add an inbox to an agent
// @Description Creates a stable inbox address when the agent does not have one. Repeated requests return the existing inbox.
// @Tags agents
// @Produce json
// @Param id path string true "Agent ID"
// @Success 200 {object} agentInboxResponse
// @Success 201 {object} agentInboxResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 503 {object} errorResponse
// @Security BearerAuth
// @Router /v1/agents/{id}/inbox [post]
func (h *AgentHandler) ProvisionInbox(w http.ResponseWriter, r *http.Request) {
	agent, ok := h.authorizeAgentInbox(w, r)
	if !ok {
		return
	}
	inbox, created, err := agentemail.ProvisionInbox(
		r.Context(),
		h.db,
		*agent.OrgID,
		agent.ID,
		h.agentInboxDomain(),
	)
	if err != nil {
		writeAgentInboxError(r, w, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, toAgentInboxResponse(&inbox))
}

func (h *AgentHandler) authorizeAgentInbox(w http.ResponseWriter, r *http.Request) (model.Agent, bool) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok || org == nil {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing org context"})
		return model.Agent{}, false
	}
	agentID, ok := agentIDFromRequest(w, r)
	if !ok {
		return model.Agent{}, false
	}
	var agent model.Agent
	if err := h.db.WithContext(r.Context()).
		Where("id = ? AND org_id = ? AND status <> ?", agentID, org.ID, "archived").
		First(&agent).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeAgentInboxError(r, w, agentemail.ErrAgentNotFound)
			return model.Agent{}, false
		}
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "load agent for inbox", "error", err, "agent_id", agentID)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load agent"})
		return model.Agent{}, false
	}
	if isAPIKeyRequest(r.Context()) {
		return agent, true
	}
	rawUserID := strings.TrimSpace(middleware.UserID(r.Context()))
	if rawUserID == "" {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing auth context"})
		return model.Agent{}, false
	}
	actor, err := access.Resolve(r.Context(), h.db, org.ID, rawUserID)
	if err != nil || actor == nil {
		if err != nil {
			logging.FromContext(r.Context()).ErrorContext(r.Context(), "resolve agent inbox actor", "error", err, "agent_id", agent.ID)
		}
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "agent not found"})
		return model.Agent{}, false
	}
	allowed, err := actor.CanManageTeamResource(r.Context(), h.db, agent.TeamID)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "authorize agent inbox", "error", err, "agent_id", agent.ID)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to authorize agent"})
		return model.Agent{}, false
	}
	if !allowed {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "agent not found"})
		return model.Agent{}, false
	}
	return agent, true
}

func (h *AgentHandler) agentInboxDomain() string {
	if h == nil || h.compileDeps.Cfg == nil {
		return ""
	}
	return h.compileDeps.Cfg.AgentInboxDomain
}

func toAgentInboxResponse(inbox *agentemail.Inbox) agentInboxResponse {
	if inbox == nil {
		return agentInboxResponse{Available: false}
	}
	return agentInboxResponse{
		Available:    true,
		Email:        inbox.Address,
		MessageCount: inbox.MessageCount,
	}
}

func writeAgentInboxError(r *http.Request, w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, agentemail.ErrAgentNotFound):
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "agent not found"})
	case errors.Is(err, agentemail.ErrNotConfigured):
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "agent email is not configured"})
	default:
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "agent inbox operation failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to manage agent inbox"})
	}
}
