package handler

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
)

// ListAgentGrants handles GET /v1/agents/{agentID}/mcp-servers.
// @Summary List direct MCP server grants and overrides for an agent
// @Tags mcp-servers
// @Produce json
// @Param agentID path string true "Agent ID"
// @Success 200 {object} mcpAgentGrantsResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/agents/{agentID}/mcp-servers [get]
func (h *MCPServerHandler) ListAgentGrants(w http.ResponseWriter, r *http.Request) {
	org, actor, _, ok := h.requestContext(w, r, false)
	if !ok {
		return
	}
	agentID, teamID, ok := h.loadAccessibleAgent(w, r, org.ID, actor)
	if !ok {
		return
	}
	_ = teamID
	grants, err := h.service.AgentGrants(r.Context(), org.ID, agentID)
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

// SetAgentGrant handles PUT /v1/agents/{agentID}/mcp-servers.
// @Summary Set a direct MCP grant or inherited-server disable for an agent
// @Tags mcp-servers
// @Accept json
// @Produce json
// @Param agentID path string true "Agent ID"
// @Param body body mcpAgentGrantRequest true "Agent MCP grant"
// @Success 200 {object} mcpAgentGrantsResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 422 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/agents/{agentID}/mcp-servers [put]
func (h *MCPServerHandler) SetAgentGrant(w http.ResponseWriter, r *http.Request) {
	org, actor, userID, ok := h.requestContext(w, r, true)
	if !ok {
		return
	}
	if actor == nil || !actor.IsOrgManager() {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "organization manager access required"})
		return
	}
	agentID, _, ok := h.loadAccessibleAgent(w, r, org.ID, actor)
	if !ok {
		return
	}
	var request mcpAgentGrantRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	serverID, err := uuid.Parse(request.MCPServerID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid mcp_server_id"})
		return
	}
	if err := h.service.SetAgentGrant(r.Context(), org.ID, agentID, serverID, request.State, userID); err != nil {
		writeMCPError(w, r, err)
		return
	}
	h.listAgentGrants(w, r, org.ID, agentID)
}

// DeleteAgentGrant handles DELETE /v1/agents/{agentID}/mcp-servers/{serverID}.
// @Summary Delete a direct MCP grant or override
// @Tags mcp-servers
// @Produce json
// @Param agentID path string true "Agent ID"
// @Param serverID path string true "MCP server ID"
// @Success 200 {object} statusResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/agents/{agentID}/mcp-servers/{serverID} [delete]
func (h *MCPServerHandler) DeleteAgentGrant(w http.ResponseWriter, r *http.Request) {
	org, actor, _, ok := h.requestContext(w, r, true)
	if !ok {
		return
	}
	if actor == nil || !actor.IsOrgManager() {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "organization manager access required"})
		return
	}
	agentID, _, ok := h.loadAccessibleAgent(w, r, org.ID, actor)
	if !ok {
		return
	}
	serverID, ok := mcpPathID(w, r, "serverID")
	if !ok {
		return
	}
	if err := h.service.DeleteAgentGrant(r.Context(), org.ID, agentID, serverID); err != nil {
		writeMCPError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, statusResponse{Status: "deleted"})
}
