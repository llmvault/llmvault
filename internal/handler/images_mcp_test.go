package handler_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/model"
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
