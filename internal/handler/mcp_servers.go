package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/mcpservers"
	"github.com/usehivy/hivy/internal/model"
)

type MCPServerHandler struct {
	db          *gorm.DB
	service     *mcpservers.Service
	frontendURL string
}

func NewMCPServerHandler(db *gorm.DB, service *mcpservers.Service, frontendURL string) *MCPServerHandler {
	return &MCPServerHandler{db: db, service: service, frontendURL: strings.TrimRight(frontendURL, "/")}
}

func (h *MCPServerHandler) Mount(r chi.Router) {
	r.Get("/mcp-servers", h.List)
	r.Post("/mcp-servers", h.Create)
	r.Get("/mcp-servers/{id}", h.Get)
	r.Patch("/mcp-servers/{id}", h.Update)
	r.Delete("/mcp-servers/{id}", h.Delete)
	r.Put("/mcp-servers/{id}/authorization", h.UpsertAuthorization)
	r.Delete("/mcp-servers/{id}/authorization", h.DeleteAuthorization)
	r.Post("/mcp-servers/{id}/oauth/start", h.StartOAuth)
	r.Post("/mcp-servers/{id}/test", h.Test)
	r.Get("/orgs/current/teams/{teamID}/mcp-servers", h.ListTeamServers)
	r.Post("/orgs/current/teams/{teamID}/mcp-servers", h.GrantTeamServer)
	r.Delete("/orgs/current/teams/{teamID}/mcp-servers/{serverID}", h.RevokeTeamServer)
	r.Get("/agents/{agentID}/mcp-servers", h.ListAgentGrants)
	r.Put("/agents/{agentID}/mcp-servers", h.SetAgentGrant)
	r.Delete("/agents/{agentID}/mcp-servers/{serverID}", h.DeleteAgentGrant)
	r.Get("/agents/{agentID}/personal-mcp-servers", h.ListPersonalAttachments)
	r.Post("/agents/{agentID}/personal-mcp-servers", h.AttachPersonal)
	r.Delete("/agents/{agentID}/personal-mcp-servers/{serverID}", h.DetachPersonal)
}

// List handles GET /v1/mcp-servers.
// @Summary List visible MCP servers
// @Tags mcp-servers
// @Produce json
// @Param limit query int false "Page size (default 50, max 100)"
// @Param cursor query string false "Pagination cursor"
// @Success 200 {object} mcpServersResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/mcp-servers [get]
func (h *MCPServerHandler) List(w http.ResponseWriter, r *http.Request) {
	org, actor, userID, ok := h.requestContext(w, r, false)
	if !ok {
		return
	}
	limit, cursor, err := parsePagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	input := mcpservers.ListServersInput{
		Limit:      limit,
		IncludeOrg: actor != nil && actor.IsOrgManager(),
	}
	if cursor != nil {
		input.BeforeCreatedAt = &cursor.CreatedAt
		input.BeforeID = &cursor.ID
	}
	page, err := h.service.ListServers(r.Context(), org.ID, userID, input)
	if err != nil {
		writeMCPError(w, r, err)
		return
	}
	serverIDs := make([]uuid.UUID, len(page.Servers))
	for i := range page.Servers {
		serverIDs[i] = page.Servers[i].ID
	}
	includeService := actor != nil && actor.IsOrgManager()
	authorizations, err := h.service.AuthorizationSummariesForServers(r.Context(), org.ID, serverIDs, userID, includeService)
	if err != nil {
		writeMCPError(w, r, err)
		return
	}
	responses := make([]mcpServerResponse, 0, len(page.Servers))
	for _, server := range page.Servers {
		summaries := authorizations[server.ID]
		responses = append(responses, mcpServerView(server, summaries.User, summaries.Service))
	}
	response := mcpServersResponse{MCPServers: responses, HasMore: page.HasMore}
	if page.HasMore && len(page.Servers) > 0 {
		last := page.Servers[len(page.Servers)-1]
		next := encodeCursor(last.CreatedAt, last.ID)
		response.NextCursor = &next
	}
	writeJSON(w, http.StatusOK, response)
}

// Create handles POST /v1/mcp-servers.
// @Summary Add a personal or organization MCP server
// @Tags mcp-servers
// @Accept json
// @Produce json
// @Param body body createMCPServerRequest true "MCP server"
// @Success 201 {object} mcpServerEnvelope
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Failure 422 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/mcp-servers [post]
func (h *MCPServerHandler) Create(w http.ResponseWriter, r *http.Request) {
	org, actor, userID, ok := h.requestContext(w, r, true)
	if !ok {
		return
	}
	var request createMCPServerRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	if request.Scope == model.MCPServerScopeOrg && (actor == nil || !actor.IsOrgManager()) {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "organization manager access required"})
		return
	}
	var authorization *mcpservers.AuthorizationInput
	if request.Authorization != nil {
		value := mcpAuthorizationInput(*request.Authorization)
		authorization = &value
		if value.PrincipalType == model.MCPPrincipalOrgService && (actor == nil || !actor.IsOrgManager()) {
			writeJSON(w, http.StatusForbidden, errorResponse{Error: "organization manager access required"})
			return
		}
	}
	server, err := h.service.CreateServer(r.Context(), org.ID, *userID, mcpservers.CreateServerParams{
		Scope: request.Scope, Name: request.Name, Slug: request.Slug,
		Description: request.Description, URL: request.URL, Transport: request.Transport,
		AuthType: request.AuthType, AuthorizationPolicy: request.AuthorizationPolicy,
		HeaderName: request.HeaderName, OAuthMetadata: request.OAuthMetadata,
		Authorization: authorization,
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
	writeJSON(w, http.StatusCreated, mcpServerEnvelope{MCPServer: response})
}

// Get handles GET /v1/mcp-servers/{id}.
// @Summary Get an MCP server
// @Tags mcp-servers
// @Produce json
// @Param id path string true "MCP server ID"
// @Success 200 {object} mcpServerEnvelope
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/mcp-servers/{id} [get]
func (h *MCPServerHandler) Get(w http.ResponseWriter, r *http.Request) {
	org, actor, userID, ok := h.requestContext(w, r, false)
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
	response, err := h.view(r, *server, actor, userID)
	if err != nil {
		writeMCPError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, mcpServerEnvelope{MCPServer: response})
}

// Update handles PATCH /v1/mcp-servers/{id}.
// @Summary Update an MCP server
// @Tags mcp-servers
// @Accept json
// @Produce json
// @Param id path string true "MCP server ID"
// @Param body body updateMCPServerRequest true "MCP server update"
// @Success 200 {object} mcpServerEnvelope
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Failure 422 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/mcp-servers/{id} [patch]
