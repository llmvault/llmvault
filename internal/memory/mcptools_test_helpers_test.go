package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/testdb"
)

type memoryToolFixture struct {
	org       model.Org
	user      model.User
	otherUser model.User
	agent     model.Agent
	channel   model.Channel
	session   model.Session
	token     *model.Token
}

func connectMemoryToolTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(postgres.Open(testdb.DatabaseURL()), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect test db: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(3)
	sqlDB.SetMaxIdleConns(1)
	testdb.ApplyMigrations(t, db)
	t.Cleanup(func() { sqlDB.Close() })
	return db
}

func seedMemoryToolFixture(t *testing.T, db *gorm.DB) memoryToolFixture {
	t.Helper()
	org := model.Org{ID: uuid.New(), Name: "memory-mcp-" + uuid.NewString(), Active: true, RateLimit: 1000}
	user := model.User{ID: uuid.New(), Email: "memory-mcp-" + uuid.NewString() + "@example.com", Name: "Memory MCP"}
	otherUser := model.User{ID: uuid.New(), Email: "memory-mcp-other-" + uuid.NewString() + "@example.com", Name: "Other Memory MCP"}
	agent := model.Agent{ID: uuid.New(), OrgID: &org.ID, Name: "Memory MCP Agent " + uuid.NewString(), Model: "test", Status: "active"}
	channel := model.Channel{ID: uuid.New(), OrgID: org.ID, Name: "memory-mcp-" + uuid.NewString(), DefaultAgentID: agent.ID, ExposeOrgMemories: true}
	session := model.Session{ID: uuid.New(), OrgID: org.ID, ChannelID: channel.ID, AgentID: agent.ID, CreatedBy: &user.ID, Status: "active"}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := db.Create(&otherUser).Error; err != nil {
		t.Fatalf("create other user: %v", err)
	}
	if err := db.Create(&model.OrgMembership{OrgID: org.ID, UserID: user.ID, Role: "member"}).Error; err != nil {
		t.Fatalf("create membership: %v", err)
	}
	if err := db.Create(&model.OrgMembership{OrgID: org.ID, UserID: otherUser.ID, Role: "member"}).Error; err != nil {
		t.Fatalf("create other membership: %v", err)
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if err := db.Create(&channel).Error; err != nil {
		t.Fatalf("create channel: %v", err)
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatalf("create session: %v", err)
	}
	t.Cleanup(func() {
		db.Where("org_id = ?", org.ID).Delete(&model.AgentMemory{})
		db.Where("org_id = ?", org.ID).Delete(&model.Session{})
		db.Where("org_id = ?", org.ID).Delete(&model.Channel{})
		db.Where("org_id = ?", org.ID).Delete(&model.Agent{})
		db.Where("org_id = ?", org.ID).Delete(&model.OrgMembership{})
		db.Delete(&model.Org{}, "id = ?", org.ID)
		db.Delete(&model.User{}, "id IN ?", []uuid.UUID{user.ID, otherUser.ID})
	})
	token := &model.Token{
		OrgID: org.ID,
		Meta: model.JSON{
			model.TokenMetaType:    model.TokenTypeAgentProxy,
			model.TokenMetaAgentID: agent.ID.String(),
		},
	}
	return memoryToolFixture{org: org, user: user, otherUser: otherUser, agent: agent, channel: channel, session: session, token: token}
}

func connectMemoryToolClient(t *testing.T, ctx context.Context, service *Service, token *model.Token) *mcp.ClientSession {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "hivy-test", Version: "v1"}, nil)
	NewToolsFunc(service)(server, token)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
		t.Fatalf("connect server: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "memory-mcp-test", Version: "v1"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	return session
}

func assertMemoryToolDescriptions(t *testing.T, tools []*mcp.Tool) {
	t.Helper()
	byName := map[string]*mcp.Tool{}
	for _, tool := range tools {
		byName[tool.Name] = tool
	}
	for _, name := range []string{"search_memories"} {
		if byName[name] == nil {
			t.Fatalf("tool %s not registered", name)
		}
		if len(byName[name].Description) < 120 {
			t.Fatalf("tool %s has weak description %q", name, byName[name].Description)
		}
		if !strings.Contains(byName[name].Description, "channel") {
			t.Fatalf("tool %s description does not mention channel scoping: %q", name, byName[name].Description)
		}
	}
	assertMemoryToolSchemas(t, tools)
}

func assertMemoryToolSchemas(t *testing.T, tools []*mcp.Tool) {
	t.Helper()
	for _, tool := range tools {
		schema, err := json.Marshal(tool.InputSchema)
		if err != nil {
			t.Fatalf("marshal tool schema: %v", err)
		}
		if strings.Contains(string(schema), "_hivy_session_id") {
			t.Fatalf("tool %s exposes _hivy_session_id in schema: %s", tool.Name, schema)
		}
		if tool.Name == "search_memories" && strings.Contains(string(schema), "limit") {
			t.Fatalf("search_memories exposes agent-controlled limit: %s", schema)
		}
		// Regular agent tools must not carry any scope/owner concept anymore.
		if tool.Name == "search_memories" && strings.Contains(string(schema), "target") {
			t.Fatalf("tool %s still exposes a target argument: %s", tool.Name, schema)
		}
	}
}

func callMemoryTool(t *testing.T, ctx context.Context, client *mcp.ClientSession, name string, args map[string]any) map[string]any {
	t.Helper()
	result, err := client.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("call %s: %v", name, err)
	}
	if result.IsError {
		t.Fatalf("%s returned error: %s", name, memoryToolText(result))
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(memoryToolText(result)), &out); err != nil {
		t.Fatalf("decode %s response %q: %v", name, memoryToolText(result), err)
	}
	return out
}

// seedReadyMemory stores a memory scoped to channelID (nil = org-wide) and marks
// it embedded so semantic search can reach it.
func seedReadyMemory(t *testing.T, service *Service, orgID uuid.UUID, channelID *uuid.UUID, content string) uuid.UUID {
	t.Helper()
	return seedReadyMemoryTagged(t, service, orgID, channelID, content, nil)
}

func seedReadyMemoryTagged(t *testing.T, service *Service, orgID uuid.UUID, channelID *uuid.UUID, content string, tags []string) uuid.UUID {
	t.Helper()
	mem, err := service.Create(context.Background(), CreateRequest{
		OrgID:     orgID,
		ChannelID: channelID,
		Content:   content,
		Tags:      tags,
	})
	if err != nil {
		t.Fatalf("seed memory: %v", err)
	}
	ok, err := service.MarkEmbeddingReady(context.Background(), mem.ID, mem.EmbeddingRevision, testMemoryVector())
	if err != nil || !ok {
		t.Fatalf("mark memory ready: ok=%v err=%v", ok, err)
	}
	return mem.ID
}

type staticMemoryToolEmbedder struct {
	vector []float32
}

func (e staticMemoryToolEmbedder) Embed(_ context.Context, inputs []string) ([][]float32, int, error) {
	if len(inputs) == 0 {
		return nil, 0, fmt.Errorf("inputs are required")
	}
	out := make([][]float32, len(inputs))
	for i := range inputs {
		out[i] = append([]float32(nil), e.vector...)
	}
	return out, 0, nil
}

func testMemoryVector() []float32 {
	vector := make([]float32, DefaultEmbeddingDim)
	vector[0] = 0.1
	vector[1] = 0.2
	vector[2] = 0.3
	return vector
}

func memoryToolText(result *mcp.CallToolResult) string {
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
