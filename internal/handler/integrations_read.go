package handler

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/integrations"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

func (h *IntegrationHandler) List(w http.ResponseWriter, r *http.Request) {
	limit, cursor, err := parsePagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	q := h.db.Where("custom_app = false AND deleted_at IS NULL")

	if provider := r.URL.Query().Get("provider"); provider != "" {
		q = q.Where("provider = ?", provider)
	}

	q = applyPagination(q, cursor, limit)

	var integrations []model.Integration
	if err := q.Find(&integrations).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list integrations"})
		return
	}

	hasMore := len(integrations) > limit
	if hasMore {
		integrations = integrations[:limit]
	}

	resp := make([]integrationResponse, len(integrations))
	for i, integ := range integrations {
		resp[i] = toIntegrationResponse(integ)
	}

	result := paginatedResponse[integrationResponse]{
		Data:    resp,
		HasMore: hasMore,
	}
	if hasMore {
		last := integrations[len(integrations)-1]
		c := encodeCursor(last.CreatedAt, last.ID)
		result.NextCursor = &c
	}

	writeJSON(w, http.StatusOK, result)
}

func (h *IntegrationHandler) Get(w http.ResponseWriter, r *http.Request) {
	integID := chi.URLParam(r, "id")
	if integID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "integration id required"})
		return
	}

	var integ model.Integration
	if err := h.db.Where("id = ? AND custom_app = false AND deleted_at IS NULL", integID).First(&integ).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "integration not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get integration"})
		return
	}

	nk := nangoProviderConfigKey(integ.UniqueKey)
	integResp, err := h.nango.GetIntegration(r.Context(), nk)
	if err != nil {
		logging.FromContext(r.Context()).WarnContext(r.Context(), "failed to fetch nango integration", "error", err, "integration_id", integ.ID)
	} else {
		template, _ := h.nango.GetProviderTemplate(nangoProviderName(integ.Provider))
		integ.NangoConfig = buildNangoConfig(integResp, template, h.nango.CallbackURL())
	}

	writeJSON(w, http.StatusOK, toIntegrationResponse(integ))
}

// @Summary List available platform integrations
// @Description Returns non-deleted platform integrations with safe fields for end users.
// @Tags integrations
// @Produce json
// @Success 200 {array} integrationAvailableResponse
// @Security BearerAuth
// @Router /v1/integrations/available [get]
func (h *IntegrationHandler) ListAvailable(w http.ResponseWriter, r *http.Request) {
	var integrations []model.Integration
	if err := h.db.Where("custom_app = false AND deleted_at IS NULL").Order("created_at ASC").Find(&integrations).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list integrations"})
		return
	}

	resp := make([]integrationAvailableResponse, 0, len(integrations))
	for _, integ := range integrations {
		integ = h.withAvailableNangoConfig(r.Context(), integ)
		resp = append(resp, toIntegrationAvailableResponse(integ))
	}

	writeJSON(w, http.StatusOK, resp)
}

// @Summary List supported platform integrations
// @Description Returns every enabled platform integration definition and whether it is configured for connection.
// @Tags integrations
// @Produce json
// @Success 200 {object} supportedIntegrationsResponse
// @Failure 500 {object} errorResponse
// @Router /v1/integrations/supported [get]
func (h *IntegrationHandler) ListSupported(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	definitions, err := integrations.ListSupportedDefinitions("global/integrations")
	if err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "load supported integration definitions", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to list supported integrations"})
		return
	}

	var configured []model.Integration
	if err := h.db.WithContext(ctx).
		Where("custom_app = false AND deleted_at IS NULL").
		Find(&configured).Error; err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "load configured integrations", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to list supported integrations"})
		return
	}

	byManagedID := make(map[string]model.Integration, len(configured))
	byUniqueKey := make(map[string]model.Integration, len(configured))
	for _, integration := range configured {
		byManagedID[integration.ManagedID] = integration
		byUniqueKey[integration.UniqueKey] = integration
	}

	data := make([]supportedIntegrationResponse, 0, len(definitions))
	for _, definition := range definitions {
		integration, ok := byManagedID[definition.ID]
		if !ok {
			integration, ok = byUniqueKey[definition.UniqueKey]
		}
		if !ok {
			data = append(data, supportedIntegrationResponse{
				DefinitionID: definition.ID,
				ID:           definition.ID,
				Provider:     definition.Provider,
				DisplayName:  definition.DisplayName,
				Configured:   false,
			})
			continue
		}

		integration = h.withAvailableNangoConfig(ctx, integration)
		available := toIntegrationAvailableResponse(integration)
		data = append(data, supportedIntegrationResponse{
			DefinitionID: definition.ID,
			ID:           available.ID,
			Provider:     available.Provider,
			DisplayName:  available.DisplayName,
			Meta:         available.Meta,
			NangoConfig:  available.NangoConfig,
			CreatedAt:    available.CreatedAt,
			Configured:   true,
		})
	}

	writeJSON(w, http.StatusOK, supportedIntegrationsResponse{Data: data})
}

func (h *IntegrationHandler) withAvailableNangoConfig(ctx context.Context, integ model.Integration) model.Integration {
	cfg := parseNangoConfig(integ.NangoConfig)
	if cfg != nil && cfg.AuthMode != "" {
		return integ
	}
	if h.nango == nil {
		return integ
	}

	nk := nangoProviderConfigKey(integ.UniqueKey)
	integResp, err := h.nango.GetIntegration(ctx, nk)
	if err != nil {
		logging.FromContext(ctx).WarnContext(ctx, "failed to refresh nango config for available integration", "error", err, "integration_id", integ.ID)
		return integ
	}

	template, _ := h.nango.GetProviderTemplate(nangoProviderName(integ.Provider))
	refreshed := buildNangoConfig(integResp, template, h.nango.CallbackURL())
	if len(refreshed) == 0 {
		return integ
	}
	integ.NangoConfig = refreshed
	if err := h.db.WithContext(ctx).Model(&integ).Update("nango_config", refreshed).Error; err != nil {
		logging.FromContext(ctx).WarnContext(ctx, "failed to store refreshed nango config for available integration", "error", err, "integration_id", integ.ID)
	}
	return integ
}
