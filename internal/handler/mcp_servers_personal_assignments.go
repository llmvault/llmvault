package handler

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
)

// ListPersonalAttachments handles GET /v1/agents/{agentID}/personal-mcp-servers.
// @Summary List the current user's personal MCP servers attached to an agent
// @Tags mcp-servers
// @Produce json
// @Param agentID path string true "Agent ID"
// @Success 200 {object} mcpServersResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/agents/{agentID}/personal-mcp-servers [get]
func (h *MCPServerHandler) ListPersonalAttachments(w http.ResponseWriter, r *http.Request) {
	org, actor, userID, ok := h.requestContext(w, r, true)
	if !ok {
		return
	}
	agentID, _, ok := h.loadAccessibleAgent(w, r, org.ID, actor)
	if !ok {
		return
	}
	servers, err := h.service.PersonalAgentServers(r.Context(), org.ID, *userID, agentID)
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

// AttachPersonal handles POST /v1/agents/{agentID}/personal-mcp-servers.
// @Summary Attach a personal MCP server to an agent
// @Tags mcp-servers
// @Accept json
// @Produce json
// @Param agentID path string true "Agent ID"
// @Param body body mcpServerIDRequest true "Personal MCP server"
// @Success 201 {object} mcpServersResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/agents/{agentID}/personal-mcp-servers [post]
func (h *MCPServerHandler) AttachPersonal(w http.ResponseWriter, r *http.Request) {
	org, actor, userID, ok := h.requestContext(w, r, true)
	if !ok {
		return
	}
	agentID, _, ok := h.loadAccessibleAgent(w, r, org.ID, actor)
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
	if err := h.service.AttachPersonal(r.Context(), org.ID, *userID, agentID, serverID); err != nil {
		writeMCPError(w, r, err)
		return
	}
	h.listPersonalAttachmentsStatus(w, r, org.ID, agentID, actor, userID, http.StatusCreated)
}

// DetachPersonal handles DELETE /v1/agents/{agentID}/personal-mcp-servers/{serverID}.
// @Summary Detach a personal MCP server from an agent
// @Tags mcp-servers
// @Produce json
// @Param agentID path string true "Agent ID"
// @Param serverID path string true "MCP server ID"
// @Success 200 {object} statusResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/agents/{agentID}/personal-mcp-servers/{serverID} [delete]
func (h *MCPServerHandler) DetachPersonal(w http.ResponseWriter, r *http.Request) {
	org, actor, userID, ok := h.requestContext(w, r, true)
	if !ok {
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
	if err := h.service.DetachPersonal(r.Context(), org.ID, *userID, agentID, serverID); err != nil {
		writeMCPError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, statusResponse{Status: "detached"})
}
