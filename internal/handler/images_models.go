package handler

import (
	"context"
	"net/http"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/registry"
)

type imageGenerationModelsResponse struct {
	DefaultRasterModel string         `json:"default_raster_model"`
	DefaultVectorModel string         `json:"default_vector_model"`
	Models             []modelSummary `json:"models"`
}

// ListGenerationModels handles GET /v1/images/models.
func (h *ImageDescribeHandler) ListGenerationModels(w http.ResponseWriter, r *http.Request) {
	providerIDs, err := h.imageGenerationSystemProviderIDs(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, imageDescribeError{Error: "failed to load system credential", ErrorCode: "internal_error"})
		return
	}
	if len(providerIDs) == 0 {
		writeJSON(w, http.StatusServiceUnavailable, imageDescribeError{Error: "image generation system credential unavailable", ErrorCode: "system_credential_unavailable"})
		return
	}

	reg := h.registry
	if reg == nil {
		reg = registry.Global()
	}
	routed := reg.ImageGenerationModelsForProviders(providerIDs)
	models := make([]modelSummary, 0, len(routed))
	for _, item := range routed {
		mdl := item.Model
		models = append(models, modelSummary{
			ID:               mdl.ID,
			Name:             mdl.Name,
			ProviderIDs:      item.ProviderIDs,
			Family:           mdl.Family,
			Reasoning:        mdl.Reasoning,
			ToolCall:         mdl.ToolCall,
			StructuredOutput: mdl.StructuredOutput,
			OpenWeights:      mdl.OpenWeights,
			Knowledge:        mdl.Knowledge,
			ReleaseDate:      mdl.ReleaseDate,
			Modalities:       mdl.Modalities,
			Cost:             mdl.Cost,
			Limit:            mdl.Limit,
			Status:           mdl.Status,
			Speed:            mdl.Speed,
			Description:      mdl.Description,
		})
	}

	writeJSON(w, http.StatusOK, imageGenerationModelsResponse{
		DefaultRasterModel: registry.DefaultRasterImageGenerationModelID,
		DefaultVectorModel: registry.DefaultVectorImageGenerationModelID,
		Models:             models,
	})
}

func (h *ImageDescribeHandler) imageGenerationSystemProviderIDs(ctx context.Context) ([]string, error) {
	var creds []model.Credential
	if err := h.db.WithContext(ctx).
		Where("org_id IS NULL AND revoked_at IS NULL AND provider_id IN ?", imageGenerationProviderIDs()).
		Find(&creds).Error; err != nil {
		return nil, err
	}
	available := make(map[string]bool, len(creds))
	for _, cred := range creds {
		available[cred.ProviderID] = true
	}
	providerIDs := make([]string, 0, len(available))
	for _, providerID := range imageGenerationProviderIDs() {
		if available[providerID] {
			providerIDs = append(providerIDs, providerID)
		}
	}
	return providerIDs, nil
}
