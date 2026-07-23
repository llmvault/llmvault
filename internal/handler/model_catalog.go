package handler

import (
	"net/http"
	"time"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/registry"
)

type modelCatalogResponse struct {
	Models []catalogModelResponse `json:"models"`
	Total  int                    `json:"total"`
}

type catalogModelResponse struct {
	ID               string                         `json:"id"`
	Name             string                         `json:"name"`
	Family           string                         `json:"family,omitempty"`
	Reasoning        bool                           `json:"reasoning,omitempty"`
	ToolCall         bool                           `json:"tool_call,omitempty"`
	StructuredOutput bool                           `json:"structured_output,omitempty"`
	OpenWeights      bool                           `json:"open_weights,omitempty"`
	Knowledge        string                         `json:"knowledge,omitempty"`
	ReleaseDate      string                         `json:"release_date,omitempty"`
	NewFrom          *time.Time                     `json:"new_from,omitempty" format:"date-time"`
	NewTo            *time.Time                     `json:"new_to,omitempty" format:"date-time"`
	Modalities       *registry.Modalities           `json:"modalities,omitempty"`
	Cost             *registry.Cost                 `json:"cost,omitempty"`
	PricingUnit      string                         `json:"pricing_unit,omitempty"`
	Limit            *registry.Limit                `json:"limit,omitempty"`
	Status           string                         `json:"status,omitempty"`
	Speed            string                         `json:"speed,omitempty"`
	Tier             string                         `json:"tier,omitempty"`
	Description      string                         `json:"description,omitempty"`
	Providers        []catalogModelProviderResponse `json:"providers"`
}

type catalogModelProviderResponse struct {
	ID                  string               `json:"id"`
	Name                string               `json:"name"`
	API                 string               `json:"api,omitempty"`
	DocumentationURL    string               `json:"documentation_url,omitempty"`
	UpstreamModelID     string               `json:"upstream_model_id"`
	ModelName           string               `json:"model_name"`
	Priority            int                  `json:"priority"`
	Default             bool                 `json:"default"`
	Available           bool                 `json:"available"`
	Reasoning           bool                 `json:"reasoning,omitempty"`
	ToolCall            bool                 `json:"tool_call,omitempty"`
	StructuredOutput    bool                 `json:"structured_output,omitempty"`
	Modalities          *registry.Modalities `json:"modalities,omitempty"`
	Cost                *registry.Cost       `json:"cost,omitempty"`
	PricingUnit         string               `json:"pricing_unit,omitempty"`
	Limit               *registry.Limit      `json:"limit,omitempty"`
	Status              string               `json:"status,omitempty"`
	FallbackCanonicalID string               `json:"fallback_canonical_id,omitempty"`
}

// CatalogModels handles GET /v1/catalog/models.
// @Summary List the complete model catalog
// @Description Returns every canonical Hivy model with its ordered provider routes, provider-specific pricing and limits, and current system availability.
// @Tags providers
// @Produce json
// @Success 200 {object} modelCatalogResponse
// @Failure 500 {object} errorResponse
// @Router /v1/catalog/models [get]
func (h *ProviderHandler) CatalogModels(w http.ResponseWriter, r *http.Request) {
	availableProviders, err := h.availableSystemProviders(r)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(
			r.Context(),
			"load model catalog provider availability",
			"error", err,
		)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load model catalog"})
		return
	}

	catalog := h.reg.CatalogModels()
	models := make([]catalogModelResponse, 0, len(catalog))
	for _, catalogModel := range catalog {
		providers := make([]catalogModelProviderResponse, 0, len(catalogModel.Routes))
		for i, route := range catalogModel.Routes {
			providerModel := route.Model
			providers = append(providers, catalogModelProviderResponse{
				ID:                  route.ProviderID,
				Name:                route.ProviderName,
				API:                 route.ProviderAPI,
				DocumentationURL:    route.ProviderDoc,
				UpstreamModelID:     route.UpstreamModelID,
				ModelName:           providerModel.Name,
				Priority:            i + 1,
				Default:             i == 0,
				Available:           availableProviders[route.ProviderID],
				Reasoning:           providerModel.Reasoning,
				ToolCall:            providerModel.ToolCall,
				StructuredOutput:    providerModel.StructuredOutput,
				Modalities:          providerModel.Modalities,
				Cost:                providerModel.Cost,
				PricingUnit:         pricingUnit(providerModel.Cost),
				Limit:               providerModel.Limit,
				Status:              providerModel.Status,
				FallbackCanonicalID: route.CanonicalModelID,
			})
		}

		canonical := catalogModel.Model
		models = append(models, catalogModelResponse{
			ID:               canonical.ID,
			Name:             canonical.Name,
			Family:           canonical.Family,
			Reasoning:        canonical.Reasoning,
			ToolCall:         canonical.ToolCall,
			StructuredOutput: canonical.StructuredOutput,
			OpenWeights:      canonical.OpenWeights,
			Knowledge:        canonical.Knowledge,
			ReleaseDate:      canonical.ReleaseDate,
			NewFrom:          canonical.NewFrom,
			NewTo:            canonical.NewTo,
			Modalities:       canonical.Modalities,
			Cost:             canonical.Cost,
			PricingUnit:      pricingUnit(canonical.Cost),
			Limit:            canonical.Limit,
			Status:           canonical.Status,
			Speed:            canonical.Speed,
			Tier:             registry.DerivedTier(canonical),
			Description:      canonical.Description,
			Providers:        providers,
		})
	}
	writeJSON(w, http.StatusOK, modelCatalogResponse{
		Models: models,
		Total:  len(models),
	})
}

func pricingUnit(cost *registry.Cost) string {
	if cost == nil {
		return ""
	}
	return "usd_per_million_tokens"
}

func (h *ProviderHandler) availableSystemProviders(r *http.Request) (map[string]bool, error) {
	var providerIDs []string
	if err := h.db.WithContext(r.Context()).
		Model(&model.Credential{}).
		Where("org_id IS NULL AND revoked_at IS NULL").
		Distinct("provider_id").
		Pluck("provider_id", &providerIDs).Error; err != nil {
		return nil, err
	}
	available := make(map[string]bool, len(providerIDs))
	for _, providerID := range providerIDs {
		available[providerID] = true
	}
	return available, nil
}
