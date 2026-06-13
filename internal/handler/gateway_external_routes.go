package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/usehivy/hivy/internal/gateway"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

// CreateRoute handles POST /v1/agents/{id}/gateway-routes.
// @Summary Create an external gateway route
// @Tags agents
// @Accept json
// @Produce json
// @Param id path string true "Agent ID"
// @Param body body createGatewayRouteRequest true "Gateway route"
// @Success 201 {object} gatewayRouteResponse
// @Security BearerAuth
// @Router /v1/agents/{id}/gateway-routes [post]
func (h *GatewayExternalHandler) CreateRoute(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeMissingOrg(w)
		return
	}
	agent, ok := h.loadAgent(w, r, org.ID)
	if !ok {
		return
	}
	var req createGatewayRouteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	provider, callbackURL, err := validateGatewayRouteInput(req.Provider, req.CallbackURL)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	secret, hash, prefix, err := gateway.GenerateExternalGatewaySecret()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to generate gateway secret"})
		return
	}
	config, err := h.externalRouteConfig(callbackURL, hash, prefix, secret, time.Now().UTC())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to secure gateway secret"})
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	route := model.AgentGatewayRoute{
		OrgID:    org.ID,
		AgentID:  agent.ID,
		Provider: provider,
		Name:     strings.TrimSpace(req.Name),
		Enabled:  enabled,
		Config:   config,
	}
	if route.Name == "" {
		route.Name = provider
	}
	if err := h.db.WithContext(r.Context()).Create(&route).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create gateway route"})
		return
	}
	writeJSON(w, http.StatusCreated, h.routeResponse(route, secret))
}

// ListRoutes handles GET /v1/agents/{id}/gateway-routes.
// @Summary List external gateway routes
// @Tags agents
// @Produce json
// @Param id path string true "Agent ID"
// @Success 200 {array} gatewayRouteResponse
// @Security BearerAuth
// @Router /v1/agents/{id}/gateway-routes [get]
func (h *GatewayExternalHandler) ListRoutes(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeMissingOrg(w)
		return
	}
	agent, ok := h.loadAgent(w, r, org.ID)
	if !ok {
		return
	}
	var routes []model.AgentGatewayRoute
	if err := h.db.WithContext(r.Context()).
		Where("org_id = ? AND agent_id = ? AND config->>'adapter' = ? AND revoked_at IS NULL", org.ID, agent.ID, gateway.ExternalAdapterName).
		Order("created_at DESC").
		Find(&routes).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list gateway routes"})
		return
	}
	resp := make([]gatewayRouteResponse, 0, len(routes))
	for _, route := range routes {
		resp = append(resp, h.routeResponse(route, ""))
	}
	writeJSON(w, http.StatusOK, resp)
}

// GetRoute handles GET /v1/agents/{id}/gateway-routes/{routeID}.
// @Summary Get an external gateway route
// @Tags agents
// @Produce json
// @Param id path string true "Agent ID"
// @Param routeID path string true "Gateway route ID"
// @Success 200 {object} gatewayRouteResponse
// @Security BearerAuth
// @Router /v1/agents/{id}/gateway-routes/{routeID} [get]
func (h *GatewayExternalHandler) GetRoute(w http.ResponseWriter, r *http.Request) {
	route, ok := h.loadRouteForAgent(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, h.routeResponse(route, ""))
}
