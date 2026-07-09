package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/registry"
)

func TestImageGenerationMCPToolsRegisterForAgentProxyToken(t *testing.T) {
	ctx := context.Background()
	db := connectTestDB(t)
	orgID := uuid.New()
	agentID := uuid.New()
	sandboxID := uuid.New()
	token := imageGenerationMCPToken(orgID, agentID, sandboxID)
	server := mcp.NewServer(&mcp.Implementation{Name: "hivy-test", Version: "v1"}, nil)
	uploads := handler.NewUploadsHandler(db, nil)

	uploads.RegisterImageGenerationMCPTools(server, token)

	session := connectImageGenerationMCPClient(t, ctx, server)
	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	byName := map[string]*mcp.Tool{}
	for _, tool := range tools.Tools {
		byName[tool.Name] = tool
	}
	for _, name := range []string{"generate_image", "generate_vector_image", "remix_image"} {
		tool := byName[name]
		if tool == nil {
			t.Fatalf("tool %s not registered", name)
		}
		schema, err := json.Marshal(tool.InputSchema)
		if err != nil {
			t.Fatalf("marshal schema: %v", err)
		}
		if strings.Contains(string(schema), "_hivy_session_id") {
			t.Fatalf("tool %s exposes _hivy_session_id: %s", name, schema)
		}
		if name == "remix_image" {
			if !strings.Contains(string(schema), `"required":["reference_asset_ids"]`) || !strings.Contains(string(schema), `"minItems":1`) {
				t.Fatalf("remix_image schema must require reference_asset_ids with minItems 1: %s", schema)
			}
			if strings.Contains(string(schema), `"type":{`) {
				t.Fatalf("remix_image schema must not expose the type hint property: %s", schema)
			}
		}
	}
}

func TestImageGenerationMCPToolReportsUnconfiguredBackend(t *testing.T) {
	ctx := context.Background()
	db := connectTestDB(t)
	org := createTestOrg(t, db)
	agent := model.Agent{ID: uuid.New(), OrgID: &org.ID, TeamID: firstTeamID(t, db, org.ID), Name: "image-mcp-" + uuid.NewString(), Model: "test", Status: "active"}
	sandboxID := uuid.New()
	sandbox := model.Sandbox{
		ID:                     sandboxID,
		OrgID:                  &org.ID,
		AgentID:                &agent.ID,
		ExternalID:             "image-mcp-" + uuid.NewString(),
		RuntimeURL:             "https://runtime.example.test",
		EncryptedRuntimeSecret: []byte("enc"),
		Status:                 "running",
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if err := db.Create(&sandbox).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	t.Cleanup(func() {
		db.Delete(&model.Sandbox{}, "id = ?", sandbox.ID)
		db.Delete(&model.Agent{}, "id = ?", agent.ID)
	})

	token := imageGenerationMCPToken(org.ID, agent.ID, sandbox.ID)
	server := mcp.NewServer(&mcp.Implementation{Name: "hivy-test", Version: "v1"}, nil)
	uploads := handler.NewUploadsHandler(db, nil)
	uploads.RegisterImageGenerationMCPTools(server, token)
	session := connectImageGenerationMCPClient(t, ctx, server)

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "generate_image",
		Arguments: map[string]any{"prompt": "a calm product icon"},
	})
	if err != nil {
		t.Fatalf("call generate_image: %v", err)
	}
	if !result.IsError {
		t.Fatalf("generate_image should report unconfigured backend")
	}
	text := imageGenerationToolText(result)
	if !strings.Contains(text, "image generation is not configured") {
		t.Fatalf("unexpected error: %s", text)
	}
}

func TestImageGenerationMCPToolUsesDefaultReveRasterModel(t *testing.T) {
	ctx := context.Background()
	h := newStreamHarness(t)
	kms := newTestKMS(t)
	resetImageGenerationMCPProviderCredentials(t, h, "reve")
	t.Cleanup(func() { resetImageGenerationMCPProviderCredentials(t, h, "reve") })
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/image/create" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer sk-fake" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		var payload struct {
			Prompt      string `json:"prompt"`
			Version     string `json:"version"`
			AspectRatio string `json:"aspect_ratio"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode reve payload: %v", err)
		}
		if !strings.Contains(payload.Prompt, "blue square") || payload.Version != "latest" || payload.AspectRatio != "1:1" {
			t.Fatalf("payload = %+v", payload)
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("X-Reve-Request-Id", "req_raster")
		w.Header().Set("X-Reve-Version", "v2")
		w.Header().Set("X-Reve-Credits-Used", "1")
		_, _ = w.Write(tinyPNGBytes(t))
	}))
	defer upstream.Close()
	seedSystemCredential(t, h.db, kms, upstream.URL, "reve")
	h.publicAsset.WithImageGeneration(kms, registry.Global(), upstream.Client())

	result := callImageGenerationMCPTool(t, ctx, h, "generate_image", map[string]any{
		"prompt":       "minimal flat icon of a blue square",
		"aspect_ratio": "1:1",
		"count":        1,
	})
	assertImageGenerationMCPAsset(t, h, result, "raster", "reve", registry.DefaultRasterImageGenerationModelID, "reve-image")
}

func TestImageGenerationMCPToolUsesDefaultQuiverVectorModel(t *testing.T) {
	ctx := context.Background()
	h := newStreamHarness(t)
	kms := newTestKMS(t)
	resetImageGenerationMCPProviderCredentials(t, h, "quiver")
	t.Cleanup(func() { resetImageGenerationMCPProviderCredentials(t, h, "quiver") })
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/svgs/generations" {
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
			t.Fatalf("decode quiver payload: %v", err)
		}
		if payload.Model != "arrow-1.1" || !strings.Contains(payload.Prompt, "green circle") {
			t.Fatalf("payload = %+v", payload)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-ID", "req-mcp")
		body := `{"id":"resp_mcp","credits":20,"data":[{"mime_type":"image/svg+xml","svg":"<svg xmlns=\"http://www.w3.org/2000/svg\"></svg>"}]}`
		_, _ = w.Write([]byte(body))
	}))
	defer upstream.Close()
	seedSystemCredential(t, h.db, kms, upstream.URL, "quiver")
	h.publicAsset.WithImageGeneration(kms, registry.Global(), upstream.Client())

	result := callImageGenerationMCPTool(t, ctx, h, "generate_vector_image", map[string]any{
		"description":  "minimal vector logo of a green circle",
		"aspect_ratio": "1:1",
		"count":        1,
	})
	assertImageGenerationMCPAsset(t, h, result, "vector", "quiver", registry.DefaultVectorImageGenerationModelID, "arrow-1.1")
}
