package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/usehivy/hivy/internal/registry"
)

func TestImageGenerationMCPToolSurfacesReveParameterErrors(t *testing.T) {
	ctx := context.Background()
	h := newStreamHarness(t)
	kms := newTestKMS(t)
	resetImageGenerationMCPProviderCredentials(t, h, "reve")
	t.Cleanup(func() { resetImageGenerationMCPProviderCredentials(t, h, "reve") })
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode reve payload: %v", err)
		}
		if _, ok := payload["aspect_ratio"]; ok {
			t.Fatalf("blank aspect ratio should be omitted, payload=%+v", payload)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Reve-Request-Id", "req_invalid")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error_code": "INVALID_PARAMETER_VALUE",
			"message":    "One of the request parameters has an invalid value.",
			"params": map[string]any{
				"aspect_ratio": "must be one of 16:9, 9:16, 3:2, 2:3, 4:3, 3:4, or 1:1",
			},
		})
	}))
	defer upstream.Close()
	seedSystemCredential(t, h.db, kms, upstream.URL, "reve")
	h.publicAsset.WithImageGeneration(kms, registry.Global(), upstream.Client())

	token := imageGenerationMCPToken(h.orgID, h.agentID, h.sandboxID)
	server := mcp.NewServer(&mcp.Implementation{Name: "hivy-test", Version: "v1"}, nil)
	h.publicAsset.RegisterImageGenerationMCPTools(server, token)
	session := connectImageGenerationMCPClient(t, ctx, server)
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "generate_image",
		Arguments: map[string]any{
			"prompt": "minimal flat icon",
			"count":  1,
		},
	})
	if err != nil {
		t.Fatalf("call generate_image: %v", err)
	}
	if !result.IsError {
		t.Fatal("generate_image should return an MCP tool error")
	}
	text := imageGenerationToolText(result)
	for _, want := range []string{
		"INVALID_PARAMETER_VALUE",
		"One of the request parameters has an invalid value.",
		"aspect_ratio",
		"16:9",
		"request_id req_invalid",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("tool error missing %q: %s", want, text)
		}
	}
	if strings.TrimSpace(text) == "Error: image generation failed" {
		t.Fatalf("tool returned generic error: %s", text)
	}
}
