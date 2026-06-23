package handler

import (
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/registry"
)

func TestResolveImageModelPreferencePrecedence(t *testing.T) {
	agent := &model.Agent{ID: uuid.New(), ImageModel: "riverflow-v2.5-fast", VectorImageModel: "recraft-v4.1-pro-vector"}
	channel := &model.Channel{ID: uuid.New(), ImageModel: "riverflow-v2.5-pro"}
	session := &model.Session{ID: uuid.New()}

	modelID, source := resolveImageModelPreference(session, channel, agent, false)
	if modelID != "riverflow-v2.5-pro" || source != imageModelSourceChannel {
		t.Fatalf("raster preference = %q from %q", modelID, source)
	}

	session.ImageModel = registry.DefaultRasterImageGenerationModelID
	modelID, source = resolveImageModelPreference(session, channel, agent, false)
	if modelID != registry.DefaultRasterImageGenerationModelID || source != imageModelSourceSession {
		t.Fatalf("session raster preference = %q from %q", modelID, source)
	}

	modelID, source = resolveImageModelPreference(session, channel, agent, true)
	if modelID != "recraft-v4.1-pro-vector" || source != imageModelSourceAgent {
		t.Fatalf("vector preference = %q from %q", modelID, source)
	}
}

func TestResolveImageModelPreferenceDefaults(t *testing.T) {
	modelID, source := resolveImageModelPreference(nil, nil, nil, false)
	if modelID != registry.DefaultRasterImageGenerationModelID || source != imageModelSourceDefault {
		t.Fatalf("raster default = %q from %q", modelID, source)
	}

	modelID, source = resolveImageModelPreference(nil, nil, nil, true)
	if modelID != registry.DefaultVectorImageGenerationModelID || source != imageModelSourceDefault {
		t.Fatalf("vector default = %q from %q", modelID, source)
	}
}
