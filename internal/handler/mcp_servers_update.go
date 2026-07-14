package handler

import (
	"encoding/json"
	"net/http"

	"github.com/usehivy/hivy/internal/mcpservers"
	"github.com/usehivy/hivy/internal/model"
)

func (h *MCPServerHandler) Update(w http.ResponseWriter, r *http.Request) {
	org, actor, userID, ok := h.requestContext(w, r, true)
	if !ok {
		return
	}
	serverID, ok := mcpPathID(w, r, "id")
	if !ok {
		return
	}
	server, err := h.service.GetServer(r.Context(), org.ID, serverID, userID)
	if err != nil {
		writeMCPError(w, r, err)
		return
	}
	if server.Scope == model.MCPServerScopeOrg && (actor == nil || !actor.IsOrgManager()) {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "organization manager access required"})
		return
	}
	var request updateMCPServerRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	server, err = h.service.UpdateServer(r.Context(), org.ID, serverID, userID, mcpservers.UpdateServerParams{
		Name: request.Name, Slug: request.Slug, Description: request.Description, URL: request.URL,
		Transport: request.Transport, AuthType: request.AuthType, AuthorizationPolicy: request.AuthorizationPolicy,
		HeaderName: request.HeaderName, OAuthMetadata: request.OAuthMetadata, Status: request.Status,
	})
	if err != nil {
		writeMCPError(w, r, err)
		return
	}
	response, err := h.view(r, *server, actor, userID)
	if err != nil {
		writeMCPError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, mcpServerEnvelope{MCPServer: response})
}

// Delete handles DELETE /v1/mcp-servers/{id}.
// @Summary Delete an MCP server
// @Tags mcp-servers
// @Produce json
// @Param id path string true "MCP server ID"
// @Success 200 {object} statusResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/mcp-servers/{id} [delete]
func (h *MCPServerHandler) Delete(w http.ResponseWriter, r *http.Request) {
	org, actor, userID, ok := h.requestContext(w, r, true)
	if !ok {
		return
	}
	serverID, ok := mcpPathID(w, r, "id")
	if !ok {
		return
	}
	server, err := h.service.GetServer(r.Context(), org.ID, serverID, userID)
	if err != nil {
		writeMCPError(w, r, err)
		return
	}
	if server.Scope == model.MCPServerScopeOrg && (actor == nil || !actor.IsOrgManager()) {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "organization manager access required"})
		return
	}
	if err := h.service.DeleteServer(r.Context(), org.ID, serverID, userID); err != nil {
		writeMCPError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, statusResponse{Status: "deleted"})
}
