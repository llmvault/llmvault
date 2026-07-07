package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/access"
	"github.com/usehivy/hivy/internal/channelagents"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

type channelAgentsResponse struct {
	Data []agentResponse `json:"data"`
}

type assignChannelAgentRequest struct {
	AgentID string `json:"agent_id"`
}

type assignChannelAgentResponse struct {
	Agent agentResponse `json:"agent"`
}

// ListChannelAgents handles GET /v1/channels/{id}/agents.
// @Summary List agents assigned to a channel
// @Tags channels
// @Produce json
// @Param id path string true "Channel ID"
// @Success 200 {object} channelAgentsResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/channels/{id}/agents [get]
func (h *ChannelHandler) ListChannelAgents(w http.ResponseWriter, r *http.Request) {
	channel, _, _, ok := h.authorizeChannel(w, r, false)
	if !ok {
		return
	}
	agents, err := channelagents.ListAssigned(r.Context(), h.db, channel.OrgID, channel.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to list channel agents"})
		return
	}
	data := make([]agentResponse, 0, len(agents))
	for _, agent := range agents {
		data = append(data, toAgentResponse(agent))
	}
	writeJSON(w, http.StatusOK, channelAgentsResponse{Data: data})
}

// AssignChannelAgent handles POST /v1/channels/{id}/agents.
// @Summary Assign an agent to a channel
// @Tags channels
// @Accept json
// @Produce json
// @Param id path string true "Channel ID"
// @Param body body assignChannelAgentRequest true "Agent to assign"
// @Success 201 {object} assignChannelAgentResponse
// @Failure 400 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Failure 422 {object} errorResponse
// @Security BearerAuth
// @Router /v1/channels/{id}/agents [post]
func (h *ChannelHandler) AssignChannelAgent(w http.ResponseWriter, r *http.Request) {
	channel, userID, ok := h.channelForAgentMutation(w, r)
	if !ok {
		return
	}
	var req assignChannelAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	agentID, err := uuid.Parse(strings.TrimSpace(req.AgentID))
	if err != nil || agentID == uuid.Nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "agent_id must be a uuid"})
		return
	}
	exists, err := channelagents.IsAssignedRow(r.Context(), h.db, channel.OrgID, channel.ID, agentID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to check channel agent"})
		return
	}
	if exists {
		writeJSON(w, http.StatusConflict, errorResponse{Error: "agent is already assigned to this channel"})
		return
	}
	if err := channelagents.Assign(r.Context(), h.db, channel.OrgID, channel.ID, agentID, userID); err != nil {
		switch {
		case errors.Is(err, channelagents.ErrAgentArchived):
			writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: "agent is archived"})
		case errors.Is(err, channelagents.ErrAgentNotFound):
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "agent not found"})
		default:
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to assign agent"})
		}
		return
	}
	var agent model.Agent
	if err := h.db.WithContext(r.Context()).
		Preload("AgentCatalog").
		Where("id = ? AND org_id = ?", agentID, channel.OrgID).
		First(&agent).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load agent"})
		return
	}
	writeJSON(w, http.StatusCreated, assignChannelAgentResponse{Agent: toAgentResponse(agent)})
}

// UnassignChannelAgent handles DELETE /v1/channels/{id}/agents/{agentID}.
// @Summary Unassign an agent from a channel
// @Tags channels
// @Produce json
// @Param id path string true "Channel ID"
// @Param agentID path string true "Agent ID"
// @Success 204
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 422 {object} errorResponse
// @Security BearerAuth
// @Router /v1/channels/{id}/agents/{agentID} [delete]
func (h *ChannelHandler) UnassignChannelAgent(w http.ResponseWriter, r *http.Request) {
	channel, _, ok := h.channelForAgentMutation(w, r)
	if !ok {
		return
	}
	agentID, err := uuid.Parse(strings.TrimSpace(chi.URLParam(r, "agentID")))
	if err != nil || agentID == uuid.Nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "agent id must be a uuid"})
		return
	}
	if err := channelagents.Unassign(r.Context(), h.db, channel.OrgID, channel.ID, agentID); err != nil {
		switch {
		case errors.Is(err, channelagents.ErrDefaultAgent):
			writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: "change the channel default agent before unassigning it"})
		default:
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to unassign agent"})
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// channelForAgentMutation loads the channel and enforces that the actor is a
// member of it (403 otherwise), reusing the canonical access predicate so the
// HTTP surface and the agent path cannot drift. Returns the actor's user id
// (nil for API-key callers) for attribution.
func (h *ChannelHandler) channelForAgentMutation(w http.ResponseWriter, r *http.Request) (model.Channel, *uuid.UUID, bool) {
	ctx := r.Context()
	org, ok := orgForChannelRequest(w, ctx)
	if !ok {
		return model.Channel{}, nil, false
	}
	channelID, ok := channelIDFromRequest(w, r)
	if !ok {
		return model.Channel{}, nil, false
	}
	channel, found, err := h.loadChannel(ctx, org.ID, channelID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load channel"})
		return model.Channel{}, nil, false
	}
	if !found {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "channel not found"})
		return model.Channel{}, nil, false
	}
	if isAPIKeyRequest(ctx) {
		return channel, nil, true
	}
	actor, err := access.Resolve(ctx, h.db, org.ID, middleware.UserID(ctx))
	if err != nil {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "forbidden"})
		return model.Channel{}, nil, false
	}
	if !actor.CanUseChannel(ctx, h.db, channel) {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "join the channel before managing its agents"})
		return model.Channel{}, nil, false
	}
	var userID *uuid.UUID
	if actor != nil {
		id := actor.UserID
		userID = &id
	}
	return channel, userID, true
}
