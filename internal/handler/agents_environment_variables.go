package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/usehivy/hivy/internal/access"
	"github.com/usehivy/hivy/internal/agentenvaccess"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

type agentEnvironmentVariableResponse struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
}

type agentEnvironmentVariablesResponse struct {
	Data []agentEnvironmentVariableResponse `json:"data"`
}

type updateAgentEnvironmentVariableRequest struct {
	Enabled *bool `json:"enabled"`
}

type agentEnvironmentVariableMutationResponse struct {
	EnvironmentVariable agentEnvironmentVariableResponse `json:"environment_variable"`
}

// ListEnvironmentVariables handles GET /v1/agents/{id}/environment-variables.
// @Summary List environment variables inherited by an agent
// @Description Lists the agent's team environment variables and whether each variable is enabled for new sessions. Values are never returned.
// @Tags agents
// @Produce json
// @Param id path string true "Agent ID"
// @Success 200 {object} agentEnvironmentVariablesResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/agents/{id}/environment-variables [get]
func (h *AgentHandler) ListEnvironmentVariables(w http.ResponseWriter, r *http.Request) {
	agent, ok := h.authorizeAgentEnvironmentVariables(w, r)
	if !ok {
		return
	}
	variables, err := agentenvaccess.List(r.Context(), h.db, agent)
	if err != nil {
		writeAgentEnvironmentVariableError(r, w, err)
		return
	}
	data := make([]agentEnvironmentVariableResponse, len(variables))
	for i, variable := range variables {
		data[i] = toAgentEnvironmentVariableResponse(variable)
	}
	writeJSON(w, http.StatusOK, agentEnvironmentVariablesResponse{Data: data})
}

// UpdateEnvironmentVariableAccess handles PATCH /v1/agents/{id}/environment-variables/{name}.
// @Summary Enable or disable an inherited environment variable for an agent
// @Description Updates whether a team environment variable is included in new sessions for this agent.
// @Tags agents
// @Accept json
// @Produce json
// @Param id path string true "Agent ID"
// @Param name path string true "Environment variable name"
// @Param body body updateAgentEnvironmentVariableRequest true "Environment variable access"
// @Success 200 {object} agentEnvironmentVariableMutationResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/agents/{id}/environment-variables/{name} [patch]
func (h *AgentHandler) UpdateEnvironmentVariableAccess(w http.ResponseWriter, r *http.Request) {
	var req updateAgentEnvironmentVariableRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	if req.Enabled == nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "enabled is required"})
		return
	}
	name, err := normalizeTeamEnvName(chi.URLParam(r, "name"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	agent, ok := h.authorizeAgentEnvironmentVariables(w, r)
	if !ok {
		return
	}
	variable, err := agentenvaccess.SetEnabled(r.Context(), h.db, agent, name, *req.Enabled)
	if err != nil {
		writeAgentEnvironmentVariableError(r, w, err)
		return
	}
	writeJSON(w, http.StatusOK, agentEnvironmentVariableMutationResponse{
		EnvironmentVariable: toAgentEnvironmentVariableResponse(variable),
	})
}

func (h *AgentHandler) authorizeAgentEnvironmentVariables(w http.ResponseWriter, r *http.Request) (model.Agent, bool) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok || org == nil {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing org context"})
		return model.Agent{}, false
	}
	agentID, ok := agentIDFromRequest(w, r)
	if !ok {
		return model.Agent{}, false
	}
	agent, err := agentenvaccess.LoadAgent(r.Context(), h.db, org.ID, agentID)
	if err != nil {
		writeAgentEnvironmentVariableError(r, w, err)
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
			logging.FromContext(r.Context()).ErrorContext(r.Context(), "resolve agent environment variable actor", "error", err, "agent_id", agent.ID)
		}
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "agent not found"})
		return model.Agent{}, false
	}
	allowed, err := actor.CanManageTeamResource(r.Context(), h.db, agent.TeamID)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "authorize agent environment variables", "error", err, "agent_id", agent.ID)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to authorize agent"})
		return model.Agent{}, false
	}
	if !allowed {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "agent not found"})
		return model.Agent{}, false
	}
	return agent, true
}

func toAgentEnvironmentVariableResponse(variable agentenvaccess.Variable) agentEnvironmentVariableResponse {
	return agentEnvironmentVariableResponse{
		Name:        variable.Name,
		Description: variable.Description,
		Enabled:     variable.Enabled,
	}
}

func writeAgentEnvironmentVariableError(r *http.Request, w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, agentenvaccess.ErrAgentNotFound):
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "agent not found"})
	case errors.Is(err, agentenvaccess.ErrEnvironmentVariableNotFound):
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "environment variable not found"})
	default:
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "agent environment variable operation failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to manage environment variable access"})
	}
}
