package handler

import (
	"strings"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/registry"
)

const (
	imageModelSourceSession = "session"
	imageModelSourceAgent   = "agent"
	imageModelSourceDefault = "default"
)

func validateImageModelPreference(modelID string, vector bool) error {
	return registry.Global().ValidateImageGenerationModel(strings.TrimSpace(modelID), vector)
}

func resolveImageModelPreference(session *model.Session, agent *model.Agent, vector bool) (string, string) {
	if value := selectedImageModelPreference(session, agent, vector); value.modelID != "" {
		return value.modelID, value.source
	}
	if vector {
		return registry.DefaultVectorImageGenerationModelID, imageModelSourceDefault
	}
	return registry.DefaultRasterImageGenerationModelID, imageModelSourceDefault
}

type imageModelPreference struct {
	modelID string
	source  string
}

func selectedImageModelPreference(session *model.Session, agent *model.Agent, vector bool) imageModelPreference {
	if session != nil {
		if value := cleanImagePreference(session.ImageModel, session.VectorImageModel, vector); value != "" {
			return imageModelPreference{modelID: value, source: imageModelSourceSession}
		}
	}
	if agent != nil {
		if value := cleanImagePreference(agent.ImageModel, agent.VectorImageModel, vector); value != "" {
			return imageModelPreference{modelID: value, source: imageModelSourceAgent}
		}
	}
	return imageModelPreference{}
}

func cleanImagePreference(raster, vectorModel string, vector bool) string {
	if vector {
		return strings.TrimSpace(vectorModel)
	}
	return strings.TrimSpace(raster)
}
