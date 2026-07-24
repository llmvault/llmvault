package rag

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	coremodel "github.com/usehivy/hivy/internal/model"
	ragmodel "github.com/usehivy/hivy/internal/rag/model"
	"github.com/usehivy/hivy/internal/testdb"
)

func TestKnowledgeScope_BindsSessionAndSourcesToCallingAgentTeam(t *testing.T) {
	db := knowledgeScopeTestDB(t)
	org := coremodel.Org{ID: uuid.New(), Name: "knowledge-" + uuid.NewString()[:8], Active: true, RateLimit: 1000}
	teamA := coremodel.Team{ID: uuid.New(), OrgID: org.ID, Name: "team-a-" + uuid.NewString()[:8]}
	teamB := coremodel.Team{ID: uuid.New(), OrgID: org.ID, Name: "team-b-" + uuid.NewString()[:8]}
	agentA := knowledgeScopeAgent(org.ID, teamA.ID, "Agent A")
	agentB := knowledgeScopeAgent(org.ID, teamB.ID, "Agent B")
	sessionA := coremodel.Session{ID: uuid.New(), OrgID: org.ID, TeamID: teamA.ID, AgentID: agentA.ID}
	sessionB := coremodel.Session{ID: uuid.New(), OrgID: org.ID, TeamID: teamB.ID, AgentID: agentB.ID}
	staleTeamSession := coremodel.Session{ID: uuid.New(), OrgID: org.ID, TeamID: teamB.ID, AgentID: agentA.ID}
	sourceA := knowledgeScopeSource(org.ID, "Source A")
	sourceB := knowledgeScopeSource(org.ID, "Source B")
	for _, row := range []any{&org, &teamA, &teamB, &agentA, &agentB, &sessionA, &sessionB, &staleTeamSession, &sourceA, &sourceB} {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("seed %T: %v", row, err)
		}
	}
	for _, grant := range []*coremodel.TeamRagSource{
		{OrgID: org.ID, TeamID: teamA.ID, RagSourceID: sourceA.ID},
		{OrgID: org.ID, TeamID: teamB.ID, RagSourceID: sourceB.ID},
	} {
		if err := db.Create(grant).Error; err != nil {
			t.Fatalf("seed knowledge grant: %v", err)
		}
	}

	tokenA := &coremodel.Token{
		OrgID: org.ID,
		Meta: coremodel.JSON{
			coremodel.TokenMetaType:    coremodel.TokenTypeAgentProxy,
			coremodel.TokenMetaAgentID: agentA.ID.String(),
		},
	}
	teamID, err := resolveKnowledgeTeam(t.Context(), db, tokenA, sessionA.ID.String())
	if err != nil || teamID != teamA.ID {
		t.Fatalf("own session team = %s, error = %v; want %s", teamID, err, teamA.ID)
	}
	if _, err := resolveKnowledgeTeam(t.Context(), db, tokenA, sessionB.ID.String()); err == nil ||
		!strings.Contains(err.Error(), "not found for this agent") {
		t.Fatalf("cross-agent session should be hidden, got %v", err)
	}
	if _, err := resolveKnowledgeTeam(t.Context(), db, tokenA, staleTeamSession.ID.String()); err == nil ||
		!strings.Contains(err.Error(), "not found for this agent") {
		t.Fatalf("stale cross-team session should be hidden, got %v", err)
	}
	sourceIDs, err := teamSourceIDs(t.Context(), db, org.ID, teamID)
	if err != nil {
		t.Fatalf("load team sources: %v", err)
	}
	if len(sourceIDs) != 1 || sourceIDs[0] != sourceA.ID.String() {
		t.Fatalf("team A sources = %v, want only %s", sourceIDs, sourceA.ID)
	}
}

func knowledgeScopeAgent(orgID, teamID uuid.UUID, name string) coremodel.Agent {
	return coremodel.Agent{
		ID: uuid.New(), OrgID: &orgID, TeamID: teamID, Name: name,
		Model: "test", Status: "active",
	}
}

func knowledgeScopeSource(orgID uuid.UUID, name string) ragmodel.RAGSource {
	return ragmodel.RAGSource{
		ID: uuid.New(), OrgIDValue: orgID, KindValue: ragmodel.RAGSourceKindWebsite,
		Name: name, Status: ragmodel.RAGSourceStatusActive, Enabled: true,
		ConfigValue: coremodel.JSON{},
	}
}

func knowledgeScopeTestDB(t *testing.T) *gorm.DB {
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
