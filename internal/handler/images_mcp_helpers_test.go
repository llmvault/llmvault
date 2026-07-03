package handler_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/usehivy/hivy/internal/model"
)

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
