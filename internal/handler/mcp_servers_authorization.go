package handler

import (
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/usehivy/hivy/internal/mcpservers"
	"github.com/usehivy/hivy/internal/model"
)

// UpsertAuthorization handles PUT /v1/mcp-servers/{id}/authorization.
// @Summary Configure an MCP server authorization
// @Description Stores tokens and secrets encrypted. Secret values are never returned.
// @Tags mcp-servers
// @Accept json
// @Produce json
// @Param id path string true "MCP server ID"
// @Param body body mcpAuthorizationRequest true "Authorization"
// @Success 200 {object} mcpAuthorizationEnvelope
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 422 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/mcp-servers/{id}/authorization [put]
func (h *MCPServerHandler) UpsertAuthorization(w http.ResponseWriter, r *http.Request) {
	org, actor, userID, ok := h.requestContext(w, r, true)
	if !ok {
		return
	}
	serverID, ok := mcpPathID(w, r, "id")
	if !ok {
		return
	}
	var request mcpAuthorizationRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	server, err := h.service.GetServer(r.Context(), org.ID, serverID, userID)
	if err != nil {
		writeMCPError(w, r, err)
		return
	}
	request.PrincipalType = effectiveMCPPrincipal(*server, request.PrincipalType)
	if request.PrincipalType == model.MCPPrincipalOrgService && (actor == nil || !actor.IsOrgManager()) {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "organization manager access required"})
		return
	}
	summary, err := h.service.UpsertAuthorization(r.Context(), org.ID, serverID, *userID, mcpAuthorizationInput(request))
	if err != nil {
		writeMCPError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, mcpAuthorizationEnvelope{Authorization: *summary})
}

// DeleteAuthorization handles DELETE /v1/mcp-servers/{id}/authorization.
// @Summary Revoke an MCP server authorization
// @Tags mcp-servers
// @Produce json
// @Param id path string true "MCP server ID"
// @Param principal_type query string false "user or org_service"
// @Success 200 {object} statusResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/mcp-servers/{id}/authorization [delete]
func (h *MCPServerHandler) DeleteAuthorization(w http.ResponseWriter, r *http.Request) {
	org, actor, userID, ok := h.requestContext(w, r, true)
	if !ok {
		return
	}
	serverID, ok := mcpPathID(w, r, "id")
	if !ok {
		return
	}
	principal := r.URL.Query().Get("principal_type")
	server, err := h.service.GetServer(r.Context(), org.ID, serverID, userID)
	if err != nil {
		writeMCPError(w, r, err)
		return
	}
	principal = effectiveMCPPrincipal(*server, principal)
	if principal == model.MCPPrincipalOrgService && (actor == nil || !actor.IsOrgManager()) {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "organization manager access required"})
		return
	}
	if err := h.service.DeleteAuthorization(r.Context(), org.ID, serverID, *userID, principal); err != nil {
		writeMCPError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, statusResponse{Status: "revoked"})
}

// StartOAuth handles POST /v1/mcp-servers/{id}/oauth/start.
// @Summary Start MCP OAuth authorization with PKCE
// @Tags mcp-servers
// @Accept json
// @Produce json
// @Param id path string true "MCP server ID"
// @Param body body mcpOAuthStartRequest true "OAuth client"
// @Success 200 {object} mcpOAuthStartResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 422 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/mcp-servers/{id}/oauth/start [post]
func (h *MCPServerHandler) StartOAuth(w http.ResponseWriter, r *http.Request) {
	org, actor, userID, ok := h.requestContext(w, r, true)
	if !ok {
		return
	}
	serverID, ok := mcpPathID(w, r, "id")
	if !ok {
		return
	}
	var request mcpOAuthStartRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	server, err := h.service.GetServer(r.Context(), org.ID, serverID, userID)
	if err != nil {
		writeMCPError(w, r, err)
		return
	}
	request.PrincipalType = effectiveMCPPrincipal(*server, request.PrincipalType)
	if request.PrincipalType == model.MCPPrincipalOrgService && (actor == nil || !actor.IsOrgManager()) {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "organization manager access required"})
		return
	}
	result, err := h.service.StartOAuth(r.Context(), org.ID, serverID, *userID, mcpservers.OAuthStartParams{
		PrincipalType: request.PrincipalType, ClientID: request.ClientID,
		ClientSecret: request.ClientSecret, Scopes: request.Scopes,
		RedirectAfter: request.RedirectAfter,
	})
	if err != nil {
		writeMCPError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, mcpOAuthStartResponse{AuthorizationURL: result.AuthorizationURL, ExpiresAt: result.ExpiresAt})
}

func effectiveMCPPrincipal(server model.MCPServer, raw string) string {
	if raw != "" {
		return raw
	}
	if server.Scope == model.MCPServerScopePersonal || server.AuthorizationPolicy == model.MCPAuthorizationPolicyUserRequired {
		return model.MCPPrincipalUser
	}
	return model.MCPPrincipalOrgService
}

// OAuthCallback handles GET /v1/mcp-servers/oauth/callback.
// @Summary Complete MCP OAuth authorization
// @Tags mcp-servers
// @Produce json
// @Param state query string true "OAuth state"
// @Param code query string true "Authorization code"
// @Success 200 {object} mcpOAuthCallbackResponse
// @Failure 400 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Router /v1/mcp-servers/oauth/callback [get]
func (h *MCPServerHandler) OAuthCallback(w http.ResponseWriter, r *http.Request) {
	result, err := h.service.CompleteOAuth(r.Context(), r.URL.Query().Get("state"), r.URL.Query().Get("code"))
	if err != nil {
		writeMCPError(w, r, err)
		return
	}
	if result.RedirectAfter != "" && h.frontendURL != "" {
		target, err := url.JoinPath(h.frontendURL, result.RedirectAfter)
		if err == nil {
			http.Redirect(w, r, target, http.StatusSeeOther)
			return
		}
	}
	writeJSON(w, http.StatusOK, mcpOAuthCallbackResponse{Status: "connected", MCPServerID: result.MCPServerID.String()})
}

// OAuthClientMetadata handles GET /v1/mcp-servers/oauth/client-metadata.
// @Summary MCP OAuth client metadata document
// @Tags mcp-servers
// @Produce json
// @Success 200 {object} mcpservers.OAuthClientMetadata
// @Router /v1/mcp-servers/oauth/client-metadata [get]
func (h *MCPServerHandler) OAuthClientMetadata(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, h.service.ClientMetadata())
}
