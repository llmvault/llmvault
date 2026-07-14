package handler

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
)

// ListTeamServers handles GET /v1/orgs/current/teams/{teamID}/mcp-servers.
// @Summary List MCP servers granted to a team
// @Tags mcp-servers
// @Produce json
// @Param teamID path string true "Team ID"
// @Success 200 {object} mcpServersResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/orgs/current/teams/{teamID}/mcp-servers [get]
func (h *MCPServerHandler) ListTeamServers(w http.ResponseWriter, r *http.Request) {
	org, actor, userID, ok := h.requestContext(w, r, false)
	if !ok {
		return
	}
	teamID, ok := mcpPathID(w, r, "teamID")
	if !ok {
		return
	}
	if !h.canAccessTeam(w, r, actor, teamID) {
		return
	}
	servers, err := h.service.TeamServers(r.Context(), org.ID, teamID)
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
	writeJSON(w, http.StatusOK, mcpServersResponse{MCPServers: result})
}

// GrantTeamServer handles POST /v1/orgs/current/teams/{teamID}/mcp-servers.
// @Summary Grant an organization MCP server to a team
// @Tags mcp-servers
// @Accept json
// @Produce json
// @Param teamID path string true "Team ID"
// @Param body body mcpServerIDRequest true "MCP server"
// @Success 201 {object} mcpServersResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/orgs/current/teams/{teamID}/mcp-servers [post]
func (h *MCPServerHandler) GrantTeamServer(w http.ResponseWriter, r *http.Request) {
	org, actor, userID, ok := h.requestContext(w, r, true)
	if !ok {
		return
	}
	if actor == nil || !actor.IsOrgManager() {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "organization manager access required"})
		return
	}
	teamID, ok := mcpPathID(w, r, "teamID")
	if !ok {
		return
	}
	var request mcpServerIDRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	serverID, err := uuid.Parse(request.MCPServerID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid mcp_server_id"})
		return
	}
	if err := h.service.GrantTeam(r.Context(), org.ID, teamID, serverID, userID); err != nil {
		writeMCPError(w, r, err)
		return
	}
	h.listTeamServersStatus(w, r, org.ID, teamID, actor, userID, http.StatusCreated)
}

// RevokeTeamServer handles DELETE /v1/orgs/current/teams/{teamID}/mcp-servers/{serverID}.
// @Summary Revoke an MCP server from a team
// @Tags mcp-servers
// @Produce json
// @Param teamID path string true "Team ID"
// @Param serverID path string true "MCP server ID"
// @Success 200 {object} statusResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/orgs/current/teams/{teamID}/mcp-servers/{serverID} [delete]
func (h *MCPServerHandler) RevokeTeamServer(w http.ResponseWriter, r *http.Request) {
	org, actor, _, ok := h.requestContext(w, r, true)
	if !ok {
		return
	}
	if actor == nil || !actor.IsOrgManager() {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "organization manager access required"})
		return
	}
	teamID, ok := mcpPathID(w, r, "teamID")
	if !ok {
		return
	}
	serverID, ok := mcpPathID(w, r, "serverID")
	if !ok {
		return
	}
	if err := h.service.RevokeTeam(r.Context(), org.ID, teamID, serverID); err != nil {
		writeMCPError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, statusResponse{Status: "revoked"})
}
