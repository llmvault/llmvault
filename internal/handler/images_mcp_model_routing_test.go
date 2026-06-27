package handler_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/registry"
)

func TestImageGenerationMCPToolUsesSelectedOpenRouterRasterModel(t *testing.T) {
	ctx := context.Background()
	h := newStreamHarness(t)
	kms := newTestKMS(t)
	resetImageGenerationMCPProviderCredentials(t, h, "reve", "openrouter")
	t.Cleanup(func() { resetImageGenerationMCPProviderCredentials(t, h, "reve", "openrouter") })
	reveServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("reve should not be called for selected OpenRouter raster model")
	}))
	defer reveServer.Close()
	openRouter := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/images" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer sk-fake" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		var payload struct {
			Model  string `json:"model"`
			Prompt string `json:"prompt"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode openrouter payload: %v", err)
		}
		if payload.Model != "black-forest-labs/flux.2-klein-4b" || !strings.Contains(payload.Prompt, "orange triangle") {
			t.Fatalf("payload = %+v", payload)
		}
		w.Header().Set("Content-Type", "application/json")
		body := `{"data":[{"b64_json":"` + base64.StdEncoding.EncodeToString(tinyPNGBytes(t)) + `","content_type":"image/png"}]}`
		_, _ = w.Write([]byte(body))
	}))
	defer openRouter.Close()
	seedSystemCredential(t, h.db, kms, reveServer.URL, "reve")
	seedSystemCredential(t, h.db, kms, openRouter.URL, "openrouter")
	if err := h.db.Model(&model.Agent{}).
		Where("id = ?", h.agentID).
		Update("image_model", "flux.2-klein-4b").Error; err != nil {
		t.Fatalf("set agent image model: %v", err)
	}
	h.publicAsset.WithImageGeneration(kms, registry.Global(), openRouter.Client())

	result := callImageGenerationMCPTool(t, ctx, h, "generate_image", map[string]any{
		"prompt": "minimal flat icon of an orange triangle",
		"count":  1,
	})
	assertImageGenerationMCPAsset(t, h, result, "raster", "openrouter", "flux.2-klein-4b", "black-forest-labs/flux.2-klein-4b")
}
