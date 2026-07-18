package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

type externalResourceRouteRequest struct {
	ConnectionID string     `json:"connection_id"`
	AgentID      string     `json:"agent_id"`
	ResourceType string     `json:"resource_type"`
	ResourceKey  string     `json:"resource_key"`
	ResourceName string     `json:"resource_name"`
	ResourceURL  string     `json:"resource_url"`
	Metadata     model.JSON `json:"metadata"`
}

type externalResourceRouteUpdateRequest struct {
	AgentID      *string     `json:"agent_id,omitempty"`
	ResourceName *string     `json:"resource_name,omitempty"`
	ResourceURL  *string     `json:"resource_url,omitempty"`
	Metadata     *model.JSON `json:"metadata,omitempty"`
}

type externalResourceRouteResponse struct {
	ID           string     `json:"id"`
	TeamID       string     `json:"team_id"`
	ConnectionID string     `json:"connection_id"`
	AgentID      string     `json:"agent_id"`
	ResourceType string     `json:"resource_type"`
	ResourceKey  string     `json:"resource_key"`
	ResourceName string     `json:"resource_name"`
	ResourceURL  string     `json:"resource_url"`
	Metadata     model.JSON `json:"metadata"`
}

type externalResourceRoutesResponse struct {
	Data []externalResourceRouteResponse `json:"data"`
}

func externalResourceRouteToResponse(route model.TeamExternalResourceRoute) externalResourceRouteResponse {
	return externalResourceRouteResponse{
		ID: route.ID.String(), TeamID: route.TeamID.String(), ConnectionID: route.ConnectionID.String(),
		AgentID: route.AgentID.String(), ResourceType: route.ResourceType, ResourceKey: route.ResourceKey,
		ResourceName: route.ResourceName, ResourceURL: route.ResourceURL, Metadata: route.Metadata,
	}
}

// ListExternalResourceRoutes handles GET /v1/orgs/current/teams/{id}/external-resource-routes.
// @Summary List external resource routes
// @Tags teams
// @Produce json
// @Param id path string true "Team ID"
// @Success 200 {object} externalResourceRoutesResponse
// @Router /v1/orgs/current/teams/{id}/external-resource-routes [get]
func (h *TeamHandler) ListExternalResourceRoutes(w http.ResponseWriter, r *http.Request) {
	team, ok := h.authorizeEnvironmentVariableTeam(w, r)
	if !ok {
		return
	}
	var routes []model.TeamExternalResourceRoute
	if err := h.db.WithContext(r.Context()).Where("org_id = ? AND team_id = ?", team.OrgID, team.ID).Order("resource_name, resource_type, resource_key").Find(&routes).Error; err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "list external resource routes", "error", err, "team_id", team.ID)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load external resource routes"})
		return
	}
	out := make([]externalResourceRouteResponse, len(routes))
	for i := range routes {
		out[i] = externalResourceRouteToResponse(routes[i])
	}
	writeJSON(w, http.StatusOK, externalResourceRoutesResponse{Data: out})
}

// CreateExternalResourceRoute handles POST /v1/orgs/current/teams/{id}/external-resource-routes.
// @Summary Create external resource route
// @Tags teams
// @Accept json
// @Produce json
// @Param id path string true "Team ID"
// @Param body body externalResourceRouteRequest true "External resource route"
// @Success 201 {object} externalResourceRouteResponse
// @Router /v1/orgs/current/teams/{id}/external-resource-routes [post]
func (h *TeamHandler) CreateExternalResourceRoute(w http.ResponseWriter, r *http.Request) {
	var req externalResourceRouteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	team, ok := h.authorizeEnvironmentVariableTeam(w, r)
	if !ok {
		return
	}
	connectionID, err := uuid.Parse(strings.TrimSpace(req.ConnectionID))
	if err != nil || connectionID == uuid.Nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "connection_id must be a uuid"})
		return
	}
	agentID, err := uuid.Parse(strings.TrimSpace(req.AgentID))
	if err != nil || agentID == uuid.Nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "agent_id must be a uuid"})
		return
	}
	resourceType, resourceKey := strings.TrimSpace(req.ResourceType), strings.TrimSpace(req.ResourceKey)
	if resourceType == "" || resourceKey == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "resource_type and resource_key are required"})
		return
	}
	var connection model.Connection
	if err := h.db.WithContext(r.Context()).Preload("Integration").Where("id = ? AND org_id = ? AND revoked_at IS NULL", connectionID, team.OrgID).First(&connection).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "connection not found"})
			return
		}
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "load external route connection", "error", err, "team_id", team.ID)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to create external resource route"})
		return
	}
	var grantCount int64
	if err := h.db.WithContext(r.Context()).Model(&model.TeamConnectionGrant{}).Where("org_id = ? AND team_id = ? AND connection_id = ?", team.OrgID, team.ID, connectionID).Count(&grantCount).Error; err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "check external route connection grant", "error", err, "team_id", team.ID)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to create external resource route"})
		return
	}
	if grantCount == 0 {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "connection not found"})
		return
	}
	if err := h.validateExternalResourceRoute(r.Context(), connection, resourceType, resourceKey); err != nil {
		var validationErr *externalResourceRouteValidationError
		if errors.As(err, &validationErr) {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: validationErr.message})
			return
		}
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "validate external resource route", "error", err, "team_id", team.ID, "connection_id", connection.ID)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to validate external resource route"})
		return
	}
	var agent model.Agent
	if err := h.db.WithContext(r.Context()).Where("id = ? AND org_id = ? AND team_id = ? AND parent_agent_id IS NULL AND status <> ?", agentID, team.OrgID, team.ID, "archived").First(&agent).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "agent not found"})
			return
		}
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "load external route agent", "error", err, "team_id", team.ID)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to create external resource route"})
		return
	}
	route := model.TeamExternalResourceRoute{OrgID: team.OrgID, TeamID: team.ID, ConnectionID: connection.ID, AgentID: agent.ID, ResourceType: resourceType, ResourceKey: resourceKey, ResourceName: strings.TrimSpace(req.ResourceName), ResourceURL: strings.TrimSpace(req.ResourceURL), Metadata: req.Metadata}
	if route.Metadata == nil {
		route.Metadata = model.JSON{}
	}
	if userID, ok := currentRequestUserID(r.Context()); ok {
		route.CreatedBy = userID
	}
	if err := h.db.WithContext(r.Context()).Create(&route).Error; err != nil {
		if isDuplicateKeyError(err) {
			writeJSON(w, http.StatusConflict, errorResponse{Error: "external resource is already routed"})
			return
		}
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "create external resource route", "error", err, "team_id", team.ID)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to create external resource route"})
		return
	}
	writeJSON(w, http.StatusCreated, externalResourceRouteToResponse(route))
}

// UpdateExternalResourceRoute handles PATCH /v1/orgs/current/teams/{id}/external-resource-routes/{routeID}.
// @Summary Update external resource route
// @Tags teams
// @Accept json
// @Produce json
// @Param id path string true "Team ID"
// @Param routeID path string true "Route ID"
// @Param body body externalResourceRouteUpdateRequest true "External resource route changes"
// @Success 200 {object} externalResourceRouteResponse
// @Router /v1/orgs/current/teams/{id}/external-resource-routes/{routeID} [patch]
func (h *TeamHandler) UpdateExternalResourceRoute(w http.ResponseWriter, r *http.Request) {
	team, ok := h.authorizeEnvironmentVariableTeam(w, r)
	if !ok {
		return
	}
	routeID, err := uuid.Parse(chi.URLParam(r, "routeID"))
	if err != nil || routeID == uuid.Nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "routeID must be a uuid"})
		return
	}
	var req externalResourceRouteUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	var route model.TeamExternalResourceRoute
	if err := h.db.WithContext(r.Context()).Where("id = ? AND org_id = ? AND team_id = ?", routeID, team.OrgID, team.ID).First(&route).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "external resource route not found"})
			return
		}
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "load external resource route", "error", err, "team_id", team.ID)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to update external resource route"})
		return
	}
	updates := map[string]any{}
	if req.AgentID != nil {
		agentID, err := uuid.Parse(strings.TrimSpace(*req.AgentID))
		if err != nil || agentID == uuid.Nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "agent_id must be a uuid"})
			return
		}
		var agent model.Agent
		if err := h.db.WithContext(r.Context()).Where("id = ? AND org_id = ? AND team_id = ? AND parent_agent_id IS NULL AND status <> ?", agentID, team.OrgID, team.ID, "archived").First(&agent).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				writeJSON(w, http.StatusNotFound, errorResponse{Error: "agent not found"})
				return
			}
			logging.FromContext(r.Context()).ErrorContext(r.Context(), "load external route agent", "error", err, "team_id", team.ID)
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to update external resource route"})
			return
		}
		updates["agent_id"] = agent.ID
		route.AgentID = agent.ID
	}
	if req.ResourceName != nil {
		route.ResourceName = strings.TrimSpace(*req.ResourceName)
		updates["resource_name"] = route.ResourceName
	}
	if req.ResourceURL != nil {
		route.ResourceURL = strings.TrimSpace(*req.ResourceURL)
		updates["resource_url"] = route.ResourceURL
	}
	if req.Metadata != nil {
		route.Metadata = *req.Metadata
		if route.Metadata == nil {
			route.Metadata = model.JSON{}
		}
		updates["metadata"] = route.Metadata
	}
	if len(updates) == 0 {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "no changes provided"})
		return
	}
	if err := h.db.WithContext(r.Context()).Model(&model.TeamExternalResourceRoute{}).Where("id = ? AND org_id = ? AND team_id = ?", route.ID, team.OrgID, team.ID).Updates(updates).Error; err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "update external resource route", "error", err, "team_id", team.ID)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to update external resource route"})
		return
	}
	writeJSON(w, http.StatusOK, externalResourceRouteToResponse(route))
}

// DeleteExternalResourceRoute handles DELETE /v1/orgs/current/teams/{id}/external-resource-routes/{routeID}.
// @Summary Delete external resource route
// @Tags teams
// @Produce json
// @Param id path string true "Team ID"
// @Param routeID path string true "Route ID"
// @Success 200 {object} statusResponse
// @Router /v1/orgs/current/teams/{id}/external-resource-routes/{routeID} [delete]
func (h *TeamHandler) DeleteExternalResourceRoute(w http.ResponseWriter, r *http.Request) {
	team, ok := h.authorizeEnvironmentVariableTeam(w, r)
	if !ok {
		return
	}
	routeID, err := uuid.Parse(chi.URLParam(r, "routeID"))
	if err != nil || routeID == uuid.Nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "routeID must be a uuid"})
		return
	}
	result := h.db.WithContext(r.Context()).Where("id = ? AND org_id = ? AND team_id = ?", routeID, team.OrgID, team.ID).Delete(&model.TeamExternalResourceRoute{})
	if result.Error != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "delete external resource route", "error", result.Error, "team_id", team.ID)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to delete external resource route"})
		return
	}
	if result.RowsAffected != 1 {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "external resource route not found"})
		return
	}
	writeJSON(w, http.StatusOK, statusResponse{Status: "deleted"})
}
