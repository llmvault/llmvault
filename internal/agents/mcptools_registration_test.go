package agents

import (
	"testing"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

func TestAgentBuilderToolRegistration_DefaultHivyIsOnlyPrivilegedCaller(t *testing.T) {
	db := testDB(t)
	org := testOrg(t, db)
	team := testTeam(t, db, org.ID)
	ordinary := model.Agent{
		ID: uuid.New(), OrgID: &org.ID, TeamID: team.ID, Name: "Ordinary",
		Model: "test", Status: "active",
	}
	hivy := model.Agent{
		ID: uuid.New(), OrgID: &org.ID, TeamID: team.ID, Name: "Hivy",
		Model: "test", Status: "active", IsDefault: true,
	}
	for _, agent := range []*model.Agent{&ordinary, &hivy} {
		if err := db.Create(agent).Error; err != nil {
			t.Fatalf("create agent: %v", err)
		}
	}

	ordinaryTools := registeredAgentBuilderTools(t, db, org.ID, ordinary.ID)
	if len(ordinaryTools) != 1 || !ordinaryTools[toolListTeamSkills] {
		t.Fatalf("ordinary tools = %v, want only %s", ordinaryTools, toolListTeamSkills)
	}

	hivyTools := registeredAgentBuilderTools(t, db, org.ID, hivy.ID)
	for _, name := range []string{toolListTeamSkills, toolListAgents, toolGetAgent, toolCreateAgent, toolUpdateAgent} {
		if !hivyTools[name] {
			t.Fatalf("Hivy tools = %v, missing %s", hivyTools, name)
		}
	}
}

func registeredAgentBuilderTools(t *testing.T, db *gorm.DB, orgID, agentID uuid.UUID) map[string]bool {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	NewToolsFunc(Deps{DB: db, DefaultModel: "deepseek-v4-flash"}, "")(server, &model.Token{
		OrgID: orgID,
		Meta: model.JSON{
			model.TokenMetaType:    model.TokenTypeAgentProxy,
			model.TokenMetaAgentID: agentID.String(),
		},
	})
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
	out := make(map[string]bool, len(result.Tools))
	for _, tool := range result.Tools {
		out[tool.Name] = true
	}
	return out
}
