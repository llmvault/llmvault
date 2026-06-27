package registry

import (
	"fmt"
	"strings"
)

const (
	DefaultRasterImageGenerationModelID = "reve-image"
	DefaultVectorImageGenerationModelID = "recraft-v4.1-pro-vector"
)

func ModelSupportsTextOutput(m Model) bool {
	return modalityIncludes(m.Modalities, "output", "text")
}

func ModelSupportsImageOutput(m Model) bool {
	return modalityIncludes(m.Modalities, "output", "image")
}

func ModelSupportsRasterImageOutput(m Model) bool {
	return ModelSupportsImageOutput(m) && !ModelSupportsVectorImageOutput(m)
}

func ModelSupportsVectorImageOutput(m Model) bool {
	return ModelSupportsImageOutput(m) && strings.Contains(strings.ToLower(m.Family), "vector")
}

func (r *Registry) ValidateImageGenerationModel(modelID string, vector bool) error {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return nil
	}
	model, ok := r.CanonicalModel(modelID)
	if !ok {
		return fmt.Errorf("image model %q is not in the catalog", modelID)
	}
	if vector {
		if !ModelSupportsVectorImageOutput(model.Model) {
			return fmt.Errorf("image model %q does not support vector output", modelID)
		}
		return nil
	}
	if !ModelSupportsRasterImageOutput(model.Model) {
		return fmt.Errorf("image model %q does not support raster image output", modelID)
	}
	return nil
}

func (r *Registry) ImageGenerationModelsForProviders(providerIDs []string) []RoutedModel {
	allowed := map[string]bool{}
	for _, providerID := range providerIDs {
		allowed[providerID] = true
	}

	byID := map[string]RoutedModel{}
	for _, hivyModel := range supportedHivyModels {
		for _, route := range hivyModel.Routes {
			if !allowed[route.ProviderID] {
				continue
			}
			resolved, ok := r.ResolveModel(route.ProviderID, hivyModel.ID)
			if !ok || !ModelSupportsImageOutput(resolved.Model) {
				continue
			}
			rm := byID[hivyModel.ID]
			if rm.ID == "" {
				rm.Model = resolved.Model
				rm.ProviderIDs = []string{}
			}
			rm.ProviderIDs = appendIfMissing(rm.ProviderIDs, route.ProviderID)
			byID[hivyModel.ID] = rm
		}
	}

	out := make([]RoutedModel, 0, len(byID))
	for _, rm := range byID {
		out = append(out, rm)
	}
	sortRoutedModels(out)
	return out
}

func modalityIncludes(modalities *Modalities, side, want string) bool {
	if modalities == nil {
		return false
	}
	values := modalities.Output
	if side == "input" {
		values = modalities.Input
	}
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
