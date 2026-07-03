package handler_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/registry"
)

func TestImageGenerationMCPVectorizeRequiresReferenceAssetID(t *testing.T) {
	ctx := context.Background()
	h := newStreamHarness(t)
	kms := newTestKMS(t)
	resetImageGenerationMCPProviderCredentials(t, h, "quiver")
	t.Cleanup(func() { resetImageGenerationMCPProviderCredentials(t, h, "quiver") })
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("vectorize_image without a reference must not reach the upstream, got %s %s", r.Method, r.URL.Path)
	}))
	defer upstream.Close()
	seedSystemCredential(t, h.db, kms, upstream.URL, "quiver")
	h.publicAsset.WithImageGeneration(kms, registry.Global(), upstream.Client())

	token := imageGenerationMCPToken(h.orgID, h.agentID, h.sandboxID)
	server := mcp.NewServer(&mcp.Implementation{Name: "hivy-test", Version: "v1"}, nil)
	h.publicAsset.RegisterImageGenerationMCPTools(server, token)
	session := connectImageGenerationMCPClient(t, ctx, server)

	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "vectorize_image", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("call vectorize_image: %v", err)
	}
	if !result.IsError {
		t.Fatal("vectorize_image without a reference should return a tool error")
	}
	if text := imageGenerationToolText(result); !strings.Contains(text, "vectorize_image requires exactly one reference_asset_id") {
		t.Fatalf("vectorize_image error = %q", text)
	}
}

func TestImageGenerationMCPVectorizeConvertsRasterToSVG(t *testing.T) {
	ctx := context.Background()
	h := newStreamHarness(t)
	kms := newTestKMS(t)
	resetImageGenerationMCPProviderCredentials(t, h, "quiver")
	t.Cleanup(func() { resetImageGenerationMCPProviderCredentials(t, h, "quiver") })

	referenceBytes := tinyPNGBytes(t)
	referenceKey := "pub/e/" + h.agentID.String() + "/uploads/vectorize-ref-" + uuid.NewString() + ".png"
	presigner := newRealPresigner(t)
	stored, err := presigner.Stream(ctx, referenceKey, "image/png", bytes.NewReader(referenceBytes))
	if err != nil {
		t.Fatalf("stream reference bytes: %v", err)
	}
	reference := model.AgentAsset{
		ID:          uuid.New(),
		OrgID:       h.orgID,
		AgentID:     h.agentID,
		SandboxID:   &h.sandboxID,
		Path:        "uploads",
		Filename:    "vectorize-ref.png",
		Key:         stored.Key,
		ContentType: "image/png",
		Bytes:       int64(len(referenceBytes)),
	}
	if err := h.db.Create(&reference).Error; err != nil {
		t.Fatalf("create reference asset: %v", err)
	}
	t.Cleanup(func() { h.db.Delete(&model.AgentAsset{}, "id = ?", reference.ID) })

	svgMarkup := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><rect width="24" height="24"/></svg>`
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/svgs/vectorizations" {
			t.Fatalf("path = %s, want /v1/svgs/vectorizations", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer sk-fake" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		var payload struct {
			Model string `json:"model"`
			Image struct {
				URL    string `json:"url"`
				Base64 string `json:"base64"`
			} `json:"image"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode quiver vectorize payload: %v", err)
		}
		if payload.Model != "arrow-1.1" {
			t.Fatalf("model = %q, want arrow-1.1", payload.Model)
		}
		if payload.Image.Base64 != base64.StdEncoding.EncodeToString(referenceBytes) {
			t.Fatalf("image.base64 = %q, want base64 of the stored reference", payload.Image.Base64)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-ID", "req-vectorize")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "resp_vectorize",
			"credits": 15,
			"data":    []map[string]any{{"mime_type": "image/svg+xml", "svg": svgMarkup}},
		})
	}))
	defer upstream.Close()
	seedSystemCredential(t, h.db, kms, upstream.URL, "quiver")
	h.publicAsset.WithImageGeneration(kms, registry.Global(), upstream.Client())

	result := callImageGenerationMCPTool(t, ctx, h, "vectorize_image", map[string]any{
		"reference_asset_ids": []string{reference.ID.String()},
	})
	if result.ContentType != "image/svg+xml" {
		t.Fatalf("content type = %q, want image/svg+xml", result.ContentType)
	}
	assertImageGenerationMCPAsset(t, h, result, "vector", "quiver", registry.DefaultVectorImageGenerationModelID, "arrow-1.1")
}
