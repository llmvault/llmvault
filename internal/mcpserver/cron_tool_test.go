package mcpserver

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/testdb"
)

func TestResolveCronAgent_EnforcesCallerAndTeamScope(t *testing.T) {
	db := cronTestDB(t)
	org := model.Org{ID: uuid.New(), Name: "cron-" + uuid.NewString()[:8], RateLimit: 1000, Active: true}
	teamA := model.Team{ID: uuid.New(), OrgID: org.ID, Name: "team-a-" + uuid.NewString()[:8]}
	teamB := model.Team{ID: uuid.New(), OrgID: org.ID, Name: "team-b-" + uuid.NewString()[:8]}
	defaultHivy := cronTestAgent(org.ID, teamA.ID, "Hivy", true)
	sameTeam := cronTestAgent(org.ID, teamA.ID, "Same team", false)
	otherTeam := cronTestAgent(org.ID, teamB.ID, "Other team", false)
	ordinary := cronTestAgent(org.ID, teamA.ID, "Ordinary", false)
	for _, row := range []any{&org, &teamA, &teamB, &defaultHivy, &sameTeam, &otherTeam, &ordinary} {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("seed %T: %v", row, err)
		}
	}

	if got, errResult := resolveCronAgent(t.Context(), db, &ordinary, ordinary.ID.String()); errResult != nil || got.ID != ordinary.ID {
		t.Fatalf("ordinary self target = %#v, error = %#v", got, errResult)
	}
	if _, errResult := resolveCronAgent(t.Context(), db, &ordinary, sameTeam.ID.String()); errResult == nil ||
		!strings.Contains(cronResultText(errResult), "only manage its own") {
		t.Fatalf("ordinary cross-agent target should be rejected, got %#v", errResult)
	}
	if got, errResult := resolveCronAgent(t.Context(), db, &defaultHivy, sameTeam.ID.String()); errResult != nil || got.ID != sameTeam.ID {
		t.Fatalf("Hivy same-team target = %#v, error = %#v", got, errResult)
	}
	if _, errResult := resolveCronAgent(t.Context(), db, &defaultHivy, otherTeam.ID.String()); errResult == nil ||
		!strings.Contains(cronResultText(errResult), "not found in this team") {
		t.Fatalf("Hivy cross-team target should be hidden, got %#v", errResult)
	}

	ordinaryCron := registeredCronTool(t, db, org.ID, ordinary.ID)
	ordinaryProperties := ordinaryCron.InputSchema.(map[string]any)["properties"].(map[string]any)
	if _, ok := ordinaryProperties["agent_id"]; ok {
		t.Fatalf("ordinary cron schema exposes agent_id: %v", ordinaryProperties)
	}
	hivyCron := registeredCronTool(t, db, org.ID, defaultHivy.ID)
	hivyProperties := hivyCron.InputSchema.(map[string]any)["properties"].(map[string]any)
	if _, ok := hivyProperties["agent_id"]; !ok {
		t.Fatalf("Hivy cron schema must expose same-team agent_id: %v", hivyProperties)
	}
}

func cronTestAgent(orgID, teamID uuid.UUID, name string, isDefault bool) model.Agent {
	return model.Agent{
		ID: uuid.New(), OrgID: &orgID, TeamID: teamID, Name: name, Model: "test",
		Status: "active", IsDefault: isDefault,
	}
}

func cronTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(postgres.Open(testdb.DatabaseURL()), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect test db: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("open sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(3)
	sqlDB.SetMaxIdleConns(1)
	testdb.ApplyMigrations(t, db)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

func cronResultText(result *mcp.CallToolResult) string {
	if result == nil {
		return ""
	}
	for _, content := range result.Content {
		if text, ok := content.(*mcp.TextContent); ok {
			return text.Text
		}
	}
	return ""
}

func registeredCronTool(t *testing.T, db *gorm.DB, orgID, agentID uuid.UUID) *mcp.Tool {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	addCronTool(server, &model.Token{
		OrgID: orgID,
		Meta: model.JSON{
			model.TokenMetaType:    model.TokenTypeAgentProxy,
			model.TokenMetaAgentID: agentID.String(),
		},
	}, db)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(t.Context(), serverTransport, nil)
	if err != nil {
		t.Fatalf("connect server: %v", err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1"}, nil)
	clientSession, err := client.Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	t.Cleanup(func() { _ = clientSession.Close() })
	result, err := clientSession.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	for _, tool := range result.Tools {
		if tool.Name == "cron" {
			return tool
		}
	}
	t.Fatal("cron tool was not registered")
	return nil
}
