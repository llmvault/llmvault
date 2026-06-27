package handler_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
	for _, name := range []string{"generate_image", "generate_vector_image"} {
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
	}
}

func TestImageGenerationMCPToolReportsUnconfiguredBackend(t *testing.T) {
	ctx := context.Background()
	db := connectTestDB(t)
	org := createTestOrg(t, db)
	agent := model.Agent{ID: uuid.New(), OrgID: &org.ID, Name: "image-mcp-" + uuid.NewString(), Model: "test", Status: "active"}
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

func TestImageGenerationMCPToolUsesDefaultRecraftVectorModel(t *testing.T) {
	ctx := context.Background()
	h := newStreamHarness(t)
	kms := newTestKMS(t)
	resetImageGenerationMCPProviderCredentials(t, h, "openrouter")
	t.Cleanup(func() { resetImageGenerationMCPProviderCredentials(t, h, "openrouter") })
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/images" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer sk-fake" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		var payload struct {
			Model       string `json:"model"`
			Prompt      string `json:"prompt"`
			AspectRatio string `json:"aspect_ratio"`
			N           int    `json:"n"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode openrouter payload: %v", err)
		}
		if payload.Model != "recraft/recraft-v4.1-vector" || !strings.Contains(payload.Prompt, "green circle") || payload.AspectRatio != "1:1" || payload.N != 1 {
			t.Fatalf("payload = %+v", payload)
		}
		w.Header().Set("Content-Type", "application/json")
		body := `{"data":[{"b64_json":"` + base64.StdEncoding.EncodeToString([]byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`)) + `","media_type":"image/svg+xml"}],"usage":{"cost":0.08}}`
		_, _ = w.Write([]byte(body))
	}))
	defer upstream.Close()
	seedSystemCredential(t, h.db, kms, upstream.URL, "openrouter")
	h.publicAsset.WithImageGeneration(kms, registry.Global(), upstream.Client())

	result := callImageGenerationMCPTool(t, ctx, h, "generate_vector_image", map[string]any{
		"description":  "minimal vector logo of a green circle",
		"aspect_ratio": "1:1",
		"count":        1,
	})
	assertImageGenerationMCPAsset(t, h, result, "vector", "openrouter", registry.DefaultVectorImageGenerationModelID, "recraft/recraft-v4.1-vector")
}

func imageGenerationMCPToken(orgID, agentID, sandboxID uuid.UUID) *model.Token {
	return &model.Token{
		OrgID:     orgID,
		JTI:       uuid.NewString(),
		ExpiresAt: time.Now().Add(time.Hour),
		Meta: model.JSON{
			model.TokenMetaType:      model.TokenTypeAgentProxy,
			model.TokenMetaAgentID:   agentID.String(),
			model.TokenMetaSandboxID: sandboxID.String(),
		},
	}
}

func resetImageGenerationMCPProviderCredentials(t *testing.T, h *streamHarness, providerIDs ...string) {
	t.Helper()
	if err := h.db.Where("org_id IS NULL AND provider_id IN ?", providerIDs).Delete(&model.Credential{}).Error; err != nil {
		t.Fatalf("reset image generation credentials: %v", err)
	}
}

func callImageGenerationMCPTool(t *testing.T, ctx context.Context, h *streamHarness, name string, args map[string]any) agentImageGenerationMCPResult {
	t.Helper()
	token := imageGenerationMCPToken(h.orgID, h.agentID, h.sandboxID)
	server := mcp.NewServer(&mcp.Implementation{Name: "hivy-test", Version: "v1"}, nil)
	h.publicAsset.RegisterImageGenerationMCPTools(server, token)
	session := connectImageGenerationMCPClient(t, ctx, server)
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("call %s: %v", name, err)
	}
	if result.IsError {
		t.Fatalf("%s returned error: %s", name, imageGenerationToolText(result))
	}
	var results []agentImageGenerationMCPResult
	if err := json.Unmarshal([]byte(imageGenerationToolText(result)), &results); err != nil {
		t.Fatalf("decode %s result: %v", name, err)
	}
	if len(results) != 1 {
		t.Fatalf("%s result count = %d", name, len(results))
	}
	return results[0]
}

type agentImageGenerationMCPResult struct {
	DriveAssetID string `json:"drive_asset_id"`
	ContentType  string `json:"content_type"`
	Bytes        int64  `json:"bytes"`
	PublicURL    string `json:"public_url"`
}

func assertImageGenerationMCPAsset(t *testing.T, h *streamHarness, result agentImageGenerationMCPResult, mode, providerID, modelID, upstreamModel string) {
	t.Helper()
	if result.DriveAssetID == "" || result.Bytes <= 0 || result.PublicURL == "" || !strings.HasPrefix(result.ContentType, "image/") {
		t.Fatalf("bad image result: %+v", result)
	}
	var asset model.AgentAsset
	if err := h.db.Where("id = ? AND org_id = ? AND agent_id = ?", uuid.MustParse(result.DriveAssetID), h.orgID, h.agentID).First(&asset).Error; err != nil {
		t.Fatalf("load generated asset: %v", err)
	}
	if asset.ContentType != result.ContentType || asset.Bytes != result.Bytes {
		t.Fatalf("asset mismatch row=%+v result=%+v", asset, result)
	}
	if asset.Description == nil {
		t.Fatal("asset missing generation metadata")
	}
	var meta map[string]any
	if err := json.Unmarshal([]byte(*asset.Description), &meta); err != nil {
		t.Fatalf("decode metadata: %v", err)
	}
	if meta["mode"] != mode || meta["provider_id"] != providerID || meta["model"] != modelID || meta["upstream_model"] != upstreamModel {
		t.Fatalf("bad generation metadata: %+v", meta)
	}
}

func tinyPNGBytes(t *testing.T) []byte {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/p9sAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatalf("decode tiny png: %v", err)
	}
	return data
}

func connectImageGenerationMCPClient(t *testing.T, ctx context.Context, server *mcp.Server) *mcp.ClientSession {
	t.Helper()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
		t.Fatalf("connect server: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "image-mcp-test", Version: "v1"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	return session
}

func imageGenerationToolText(result *mcp.CallToolResult) string {
	if result == nil {
		return ""
	}
	var parts []string
	for _, item := range result.Content {
		if text, ok := item.(*mcp.TextContent); ok {
			parts = append(parts, text.Text)
		}
	}
	return strings.Join(parts, "\n")
}
