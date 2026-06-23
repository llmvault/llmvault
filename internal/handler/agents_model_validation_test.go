package handler

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/registry"
)

func TestValidateAgentSelectableModelRejectsImageOutputModel(t *testing.T) {
	h := &AgentHandler{registry: registry.Global()}

	err := h.validateAgentSelectableModel(context.Background(), uuid.New(), registry.DefaultRasterImageGenerationModelID)
	if err == nil {
		t.Fatal("expected image-output model to be rejected")
	}
	if !strings.Contains(err.Error(), "does not support text output") {
		t.Fatalf("unexpected error: %v", err)
	}
}
