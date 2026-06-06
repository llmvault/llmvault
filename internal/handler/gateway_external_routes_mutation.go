package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/usehivy/hivy/internal/gateway"
)

// UpdateRoute handles PATCH /v1/employees/{id}/gateway-routes/{routeID}.
// @Summary Update an external gateway route
// @Tags employees
// @Accept json
// @Produce json
// @Param id path string true "Employee ID"
// @Param routeID path string true "Gateway route ID"
// @Param body body updateGatewayRouteRequest true "Gateway route updates"
// @Success 200 {object} gatewayRouteResponse
// @Security BearerAuth
// @Router /v1/employees/{id}/gateway-routes/{routeID} [patch]
func (h *GatewayExternalHandler) UpdateRoute(w http.ResponseWriter, r *http.Request) {
	route, ok := h.loadRouteForEmployee(w, r)
	if !ok {
		return
	}
	var req updateGatewayRouteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	updates := map[string]any{}
	if req.Name != nil {
		updates["name"] = strings.TrimSpace(*req.Name)
	}
	if req.Provider != nil || req.CallbackURL != nil {
		provider := route.Provider
		callbackURL := externalRouteCallbackURL(route)
		if req.Provider != nil {
			provider = *req.Provider
		}
		if req.CallbackURL != nil {
			callbackURL = *req.CallbackURL
		}
		validProvider, validCallback, err := validateGatewayRouteInput(provider, callbackURL)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		route.Provider = validProvider
		route.Config["callback_url"] = validCallback
		updates["provider"] = validProvider
		updates["config"] = route.Config
	}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}
	if len(updates) == 0 {
		writeJSON(w, http.StatusOK, h.routeResponse(route, ""))
		return
	}
	if err := h.db.WithContext(r.Context()).Model(&route).Updates(updates).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update gateway route"})
		return
	}
	if err := h.db.WithContext(r.Context()).First(&route, "id = ?", route.ID).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to reload gateway route"})
		return
	}
	writeJSON(w, http.StatusOK, h.routeResponse(route, ""))
}

// DeleteRoute handles DELETE /v1/employees/{id}/gateway-routes/{routeID}.
// @Summary Revoke an external gateway route
// @Tags employees
// @Produce json
// @Param id path string true "Employee ID"
// @Param routeID path string true "Gateway route ID"
// @Success 200 {object} map[string]string
// @Security BearerAuth
// @Router /v1/employees/{id}/gateway-routes/{routeID} [delete]
func (h *GatewayExternalHandler) DeleteRoute(w http.ResponseWriter, r *http.Request) {
	route, ok := h.loadRouteForEmployee(w, r)
	if !ok {
		return
	}
	now := time.Now().UTC()
	if err := h.db.WithContext(r.Context()).Model(&route).Updates(map[string]any{
		"enabled":    false,
		"revoked_at": &now,
	}).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to revoke gateway route"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}

// RotateSecret handles POST /v1/employees/{id}/gateway-routes/{routeID}/rotate-secret.
// @Summary Rotate an external gateway route secret
// @Tags employees
// @Produce json
// @Param id path string true "Employee ID"
// @Param routeID path string true "Gateway route ID"
// @Success 200 {object} gatewayRouteResponse
// @Security BearerAuth
// @Router /v1/employees/{id}/gateway-routes/{routeID}/rotate-secret [post]
func (h *GatewayExternalHandler) RotateSecret(w http.ResponseWriter, r *http.Request) {
	route, ok := h.loadRouteForEmployee(w, r)
	if !ok {
		return
	}
	secret, hash, prefix, err := gateway.GenerateExternalGatewaySecret()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to generate gateway secret"})
		return
	}
	callbackURL := externalRouteCallbackURL(route)
	config, err := h.externalRouteConfig(callbackURL, hash, prefix, secret, time.Now().UTC())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to secure gateway secret"})
		return
	}
	if err := h.db.WithContext(r.Context()).Model(&route).Update("config", config).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to rotate gateway secret"})
		return
	}
	route.Config = config
	writeJSON(w, http.StatusOK, h.routeResponse(route, secret))
}
