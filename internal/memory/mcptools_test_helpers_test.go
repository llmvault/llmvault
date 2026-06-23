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
	channel := model.Channel{ID: uuid.New(), OrgID: org.ID, Name: "memory-mcp-" + uuid.NewString(), DefaultAgentID: agent.ID}
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
	return memoryToolFixture{org: org, user: user, otherUser: otherUser, agent: agent, session: session, token: token}
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
	for _, name := range []string{"search_memories", "retain_memory", "forget_memory"} {
		if byName[name] == nil {
			t.Fatalf("tool %s not registered", name)
		}
		if len(byName[name].Description) < 120 {
			t.Fatalf("tool %s has weak description %q", name, byName[name].Description)
		}
	}
	if !strings.Contains(byName["retain_memory"].Description, "target.owner") ||
		!strings.Contains(byName["retain_memory"].Description, "target.visibility") {
		t.Fatalf("retain_memory description does not guide target use: %q", byName["retain_memory"].Description)
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

func assertRetainedMemoryTarget(t *testing.T, db *gorm.DB, memoryID, userID, agentID uuid.UUID) {
	t.Helper()
	var mem model.AgentMemory
	if err := db.First(&mem, "id = ?", memoryID).Error; err != nil {
		t.Fatalf("load retained memory: %v", err)
	}
	if mem.Scope != model.AgentMemoryScopeUser || mem.UserID == nil || *mem.UserID != userID {
		t.Fatalf("retained user target mismatch: %#v", mem)
	}
	if mem.AgentID == nil || *mem.AgentID != agentID {
		t.Fatalf("retained agent target mismatch: %#v", mem)
	}
}

func seedReadyMemory(t *testing.T, service *Service, orgID uuid.UUID, agentID *uuid.UUID, userID *uuid.UUID, content string) uuid.UUID {
	t.Helper()
	scope := model.AgentMemoryScopeOrg
	if userID != nil {
		scope = model.AgentMemoryScopeUser
	}
	mem, err := service.Create(context.Background(), CreateRequest{
		OrgID:   orgID,
		AgentID: agentID,
		UserID:  userID,
		Scope:   scope,
		Content: content,
	})
	if err != nil {
		t.Fatalf("seed memory: %v", err)
	}
	ok, err := service.MarkEmbeddingReady(context.Background(), mem.ID, mem.EmbeddingRevision, testMemoryVector())
	if err != nil {
		t.Fatalf("mark memory ready: %v", err)
	}
	if !ok {
		t.Fatalf("mark memory ready updated no rows")
	}
	return mem.ID
}

type staticMemoryToolEmbedder struct {
	vector []float32
}

func (e staticMemoryToolEmbedder) Embed(_ context.Context, inputs []string) ([][]float32, error) {
	if len(inputs) == 0 {
		return nil, fmt.Errorf("inputs are required")
	}
	out := make([][]float32, len(inputs))
	for i := range inputs {
		out[i] = append([]float32(nil), e.vector...)
	}
	return out, nil
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
