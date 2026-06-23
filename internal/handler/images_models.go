package handler

import (
	"errors"
	"net/http"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/registry"
)

type imageGenerationModelsResponse struct {
	DefaultRasterModel string         `json:"default_raster_model"`
	DefaultVectorModel string         `json:"default_vector_model"`
	Models             []modelSummary `json:"models"`
}

// ListGenerationModels handles GET /v1/images/models.
func (h *ImageDescribeHandler) ListGenerationModels(w http.ResponseWriter, r *http.Request) {
	if _, err := h.openRouterSystemCredential(r.Context()); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusServiceUnavailable, imageDescribeError{Error: "OpenRouter system credential unavailable", ErrorCode: "system_credential_unavailable"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, imageDescribeError{Error: "failed to load system credential", ErrorCode: "internal_error"})
		return
	}

	reg := h.registry
	if reg == nil {
		reg = registry.Global()
	}
	routed := reg.ImageGenerationModelsForProviders([]string{imageDescribeProviderID})
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
