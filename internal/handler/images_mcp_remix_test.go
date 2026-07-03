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

	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/registry"
	"github.com/usehivy/hivy/internal/tasks"
)

func TestImageGenerationMCPRemixRequiresReferenceAssetIDs(t *testing.T) {
	ctx := context.Background()
	h := newStreamHarness(t)
	kms := newTestKMS(t)
	resetImageGenerationMCPProviderCredentials(t, h, "reve")
	t.Cleanup(func() { resetImageGenerationMCPProviderCredentials(t, h, "reve") })
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("remix_image without references must not reach the upstream, got %s %s", r.Method, r.URL.Path)
	}))
	defer upstream.Close()
	seedSystemCredential(t, h.db, kms, upstream.URL, "reve")
	h.publicAsset.WithImageGeneration(kms, registry.Global(), upstream.Client())

	token := imageGenerationMCPToken(h.orgID, h.agentID, h.sandboxID)
	server := mcp.NewServer(&mcp.Implementation{Name: "hivy-test", Version: "v1"}, nil)
	h.publicAsset.RegisterImageGenerationMCPTools(server, token)
	session := connectImageGenerationMCPClient(t, ctx, server)

	for name, args := range map[string]map[string]any{
		"missing": {"prompt": "remix the logo into a poster"},
		"empty":   {"prompt": "remix the logo into a poster", "reference_asset_ids": []string{}},
	} {
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "remix_image", Arguments: args})
		if err != nil {
			t.Fatalf("call remix_image (%s): %v", name, err)
		}
		if !result.IsError {
			t.Fatalf("remix_image (%s) should return a tool error", name)
		}
		text := imageGenerationToolText(result)
		if !strings.Contains(text, "remix_image requires at least one reference_asset_id") {
			t.Fatalf("remix_image (%s) error = %q, want reference_asset_id requirement", name, text)
		}
	}
}

func TestImageGenerationMCPRemixGeneratesFromDriveReference(t *testing.T) {
	ctx := context.Background()
	h := newStreamHarness(t)
	kms := newTestKMS(t)
	resetImageGenerationMCPProviderCredentials(t, h, "reve")
	t.Cleanup(func() { resetImageGenerationMCPProviderCredentials(t, h, "reve") })

	referenceBytes := tinyPNGBytes(t)
	referenceKey := "pub/e/" + h.agentID.String() + "/uploads/remix-ref-" + uuid.NewString() + ".png"
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
		Filename:    "remix-ref.png",
		Key:         stored.Key,
		ContentType: "image/png",
		Bytes:       int64(len(referenceBytes)),
	}
	if err := h.db.Create(&reference).Error; err != nil {
		t.Fatalf("create reference asset: %v", err)
	}
	t.Cleanup(func() { h.db.Delete(&model.AgentAsset{}, "id = ?", reference.ID) })

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/image/remix" {
			t.Fatalf("path = %s, want /v1/image/remix", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer sk-fake" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode reve remix payload: %v", err)
		}
		if _, ok := payload["references"]; ok {
			t.Fatalf("remix payload must not carry a references key: %#v", payload)
		}
		prompt, _ := payload["prompt"].(string)
		if !strings.Contains(prompt, "brand mascot") || payload["version"] != "latest" || payload["aspect_ratio"] != "1:1" {
			t.Fatalf("payload = %#v", payload)
		}
		images, ok := payload["reference_images"].([]any)
		if !ok || len(images) != 1 {
			t.Fatalf("reference_images = %#v, want one flat base64 string", payload["reference_images"])
		}
		if images[0] != base64.StdEncoding.EncodeToString(referenceBytes) {
			t.Fatalf("reference_images[0] = %v, want base64 of the stored reference", images[0])
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("X-Reve-Request-Id", "req_remix")
		w.Header().Set("X-Reve-Version", "v2")
		w.Header().Set("X-Reve-Credits-Used", "1")
		_, _ = w.Write(tinyPNGBytes(t))
	}))
	defer upstream.Close()
	seedSystemCredential(t, h.db, kms, upstream.URL, "reve")
	h.publicAsset.WithImageGeneration(kms, registry.Global(), upstream.Client())
	enq := &enqueue.MockClient{}
	h.publicAsset.WithUsageEnqueuer(enq)

	result := callImageGenerationMCPTool(t, ctx, h, "remix_image", map[string]any{
		"prompt":              "place the brand mascot on a beach at sunset",
		"reference_asset_ids": []string{reference.ID.String()},
		"aspect_ratio":        "1:1",
		"count":               1,
	})
	assertImageGenerationMCPAsset(t, h, result, "raster", "reve", registry.DefaultRasterImageGenerationModelID, "reve-image")

	var usageTask *tasks.ModelUsageWritePayload
	for _, task := range enq.Tasks() {
		if task.TypeName != tasks.TypeModelUsageWrite {
			continue
		}
		var payload tasks.ModelUsageWritePayload
		if err := json.Unmarshal(task.Payload, &payload); err != nil {
			t.Fatalf("decode model usage payload: %v", err)
		}
		usageTask = &payload
	}
	if usageTask == nil {
		t.Fatal("expected a model usage task for remix_image")
	}
	if usageTask.Generation.RequestPath != "/mcp/tools/remix_image" {
		t.Fatalf("usage request path = %q, want /mcp/tools/remix_image", usageTask.Generation.RequestPath)
	}
	if usageTask.Generation.ProviderID != "reve" {
		t.Fatalf("usage provider = %q, want reve", usageTask.Generation.ProviderID)
	}
}
