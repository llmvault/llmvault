package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/usehivy/hivy/internal/integrations"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/nango"
)

type upsertAdminIntegrationRequest struct {
	Credentials *nango.Credentials `json:"credentials,omitempty"`
}

type upsertAdminIntegrationResponse struct {
	State      string                       `json:"state"`
	Definition integrations.AdminDefinition `json:"definition"`
}

// ListAdmin handles GET /v1/admin/integrations.
// @Summary List admin integration definitions
// @Description Lists supported global integration definitions and existing synced records.
// @Tags admin
// @Produce json
// @Param X-Hivy-Admin-Secret header string true "Admin secret"
// @Success 200 {array} integrations.AdminDefinition
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/admin/integrations [get]
func (h *IntegrationHandler) ListAdmin(w http.ResponseWriter, r *http.Request) {
	definitions, err := integrations.NewSeeder(h.db, h.nango, h.catalog).
		ListAdminDefinitions(r.Context(), "global/integrations")
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to list admin integration definitions", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list integration definitions"})
		return
	}

	writeJSON(w, http.StatusOK, definitions)
}

// UpsertAdmin handles PUT /v1/admin/integrations/{id}.
// @Summary Sync an admin integration
// @Description Creates or updates the supported global integration in Nango and the Hivy database.
// @Tags admin
// @Accept json
// @Produce json
// @Param X-Hivy-Admin-Secret header string true "Admin secret"
// @Param id path string true "Integration definition ID"
// @Param body body upsertAdminIntegrationRequest true "Integration credentials"
// @Success 200 {object} upsertAdminIntegrationResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 502 {object} errorResponse
// @Security BearerAuth
// @Router /v1/admin/integrations/{id} [put]
func (h *IntegrationHandler) UpsertAdmin(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "integration definition id required"})
		return
	}

	var req upsertAdminIntegrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	state, definition, err := integrations.NewSeeder(h.db, h.nango, h.catalog).
		UpsertAdmin(r.Context(), id, req.Credentials)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "admin integration upsert failed", "error", err, "definition_id", id)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, upsertAdminIntegrationResponse{
		State:      state,
		Definition: definition,
	})
}

func (h *IntegrationHandler) DeleteAdmin(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "integration definition id required"})
		return
	}
	definition, err := integrations.NewSeeder(h.db, h.nango, h.catalog).DeleteAdmin(r.Context(), id)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "admin integration delete failed", "error", err, "definition_id", id)
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, definition)
}
