package handler

import (
	"errors"
	"net/http"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/access"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

func (h *MCPServerHandler) canAccessTeam(w http.ResponseWriter, r *http.Request, actor *access.Actor, teamID uuid.UUID) bool {
	var team model.Team
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok || org == nil {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing org context"})
		return false
	}
	if err := h.db.WithContext(r.Context()).Where("id = ? AND org_id = ? AND archived_at IS NULL", teamID, org.ID).First(&team).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "team not found"})
		return false
	} else if err != nil {
		writeMCPError(w, r, err)
		return false
	}
	if actor == nil {
		if _, apiKey := middleware.APIKeyClaimsFromContext(r.Context()); apiKey {
			return true
		}
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "team not found"})
		return false
	}
	allowed, err := actor.CanManageTeamResource(r.Context(), h.db, teamID)
	if err != nil {
		writeMCPError(w, r, err)
		return false
	}
	if !allowed {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "team not found"})
		return false
	}
	return true
}

func (h *MCPServerHandler) loadAccessibleAgent(w http.ResponseWriter, r *http.Request, orgID uuid.UUID, actor *access.Actor) (uuid.UUID, uuid.UUID, bool) {
	agentID, ok := mcpPathID(w, r, "agentID")
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}
	var agent model.Agent
	if err := h.db.WithContext(r.Context()).Select("id", "team_id").
		Where("id = ? AND org_id = ? AND status <> ?", agentID, orgID, "archived").First(&agent).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "agent not found"})
		return uuid.Nil, uuid.Nil, false
	} else if err != nil {
		writeMCPError(w, r, err)
		return uuid.Nil, uuid.Nil, false
	}
	if actor == nil {
		if _, apiKey := middleware.APIKeyClaimsFromContext(r.Context()); apiKey {
			return agent.ID, agent.TeamID, true
		}
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "agent not found"})
		return uuid.Nil, uuid.Nil, false
	}
	allowed, err := actor.CanManageTeamResource(r.Context(), h.db, agent.TeamID)
	if err != nil {
		writeMCPError(w, r, err)
		return uuid.Nil, uuid.Nil, false
	}
	if !allowed {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "agent not found"})
		return uuid.Nil, uuid.Nil, false
	}
	return agent.ID, agent.TeamID, true
}

func (h *MCPServerHandler) listTeamServersStatus(w http.ResponseWriter, r *http.Request, orgID, teamID uuid.UUID, actor *access.Actor, userID *uuid.UUID, status int) {
	servers, err := h.service.TeamServers(r.Context(), orgID, teamID)
	if err != nil {
		writeMCPError(w, r, err)
		return
	}
	result := make([]mcpServerResponse, 0, len(servers))
	for _, server := range servers {
		response, err := h.view(r, server, actor, userID)
		if err != nil {
			writeMCPError(w, r, err)
			return
		}
		result = append(result, response)
	}
	writeJSON(w, status, mcpServersResponse{MCPServers: result})
}

func (h *MCPServerHandler) listAgentGrants(w http.ResponseWriter, r *http.Request, orgID, agentID uuid.UUID) {
	grants, err := h.service.AgentGrants(r.Context(), orgID, agentID)
	if err != nil {
		writeMCPError(w, r, err)
		return
	}
	result := make([]mcpAgentGrantResponse, 0, len(grants))
	for _, grant := range grants {
		result = append(result, mcpAgentGrantResponse{MCPServerID: grant.MCPServerID.String(), State: grant.State})
	}
	writeJSON(w, http.StatusOK, mcpAgentGrantsResponse{MCPServers: result})
}

func (h *MCPServerHandler) listPersonalAttachmentsStatus(w http.ResponseWriter, r *http.Request, orgID, agentID uuid.UUID, actor *access.Actor, userID *uuid.UUID, status int) {
	servers, err := h.service.PersonalAgentServers(r.Context(), orgID, *userID, agentID)
	if err != nil {
		writeMCPError(w, r, err)
		return
	}
	result := make([]mcpServerResponse, 0, len(servers))
	for _, server := range servers {
		response, err := h.view(r, server, actor, userID)
		if err != nil {
			writeMCPError(w, r, err)
			return
		}
		result = append(result, response)
	}
	writeJSON(w, status, mcpServersResponse{MCPServers: result})
}
