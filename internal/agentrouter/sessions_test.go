package agentrouter

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/testdb"
)

func connectTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := testdb.DatabaseURL("DATABASE_URL", "HIVY_DATABASE_URL", "TEST_DATABASE_URL")
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("cannot connect to Postgres: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(3)
	sqlDB.SetMaxIdleConns(1)
	testdb.ApplyMigrations(t, db)
	t.Cleanup(func() { sqlDB.Close() })
	return db
}

func seedAgent(t *testing.T, db *gorm.DB, orgID, teamID uuid.UUID, name string) model.Agent {
	t.Helper()
	agent := model.Agent{
		OrgID:         &orgID,
		TeamID:        teamID,
		Name:          name,
		Model:         "deepseek-v4-flash",
		Tools:         model.JSON{},
		McpServers:    model.RawJSON("[]"),
		Skills:        model.JSON{},
		RuntimeConfig: model.JSON{},
		Permissions:   model.JSON{},
		Resources:     model.JSON{},
		Status:        "active",
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	return agent
}

func TestRecentChannelSessionsOrdersByUpdatedAtDescAndLimits(t *testing.T) {
	db := connectTestDB(t)

	org := model.Org{Name: "router-org-" + uuid.NewString()[:8], Active: true}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	team := model.Team{ID: uuid.New(), OrgID: org.ID, Name: "router-team-" + uuid.NewString()[:8]}
	if err := db.Create(&team).Error; err != nil {
		t.Fatalf("create team: %v", err)
	}
	ada := seedAgent(t, db, org.ID, team.ID, "Ada-"+uuid.NewString()[:8])
	grace := seedAgent(t, db, org.ID, team.ID, "Grace-"+uuid.NewString()[:8])

	channel := model.Channel{
		OrgID: org.ID, Name: "eng-" + uuid.NewString()[:8], Kind: "standard",
		Visibility: "public", DefaultAgentID: ada.ID, Origin: "native",
	}
	if err := db.Create(&channel).Error; err != nil {
		t.Fatalf("create channel: %v", err)
	}

	// Another channel whose sessions must NOT leak into results.
	other := model.Channel{
		OrgID: org.ID, Name: "other-" + uuid.NewString()[:8], Kind: "standard",
		Visibility: "public", DefaultAgentID: ada.ID, Origin: "native",
	}
	if err := db.Create(&other).Error; err != nil {
		t.Fatalf("create other channel: %v", err)
	}

	base := time.Now().UTC()
	mk := func(ch model.Channel, agent model.Agent, name string, updated time.Time) {
		s := model.Session{
			OrgID: org.ID, ChannelID: ch.ID, AgentID: agent.ID, Model: agent.Model,
			ReasoningEffort: "high", Source: "web", SourceResourceKey: uuid.NewString(),
			Name: name, Status: "active", AgentTurnStatus: model.SessionAgentTurnIdle,
			IntegrationScopes: model.JSON{},
		}
		if err := db.Create(&s).Error; err != nil {
			t.Fatalf("create session %s: %v", name, err)
		}
		// UpdateColumn skips gorm's automatic updated_at management.
		if err := db.Model(&model.Session{}).Where("id = ?", s.ID).
			UpdateColumn("updated_at", updated).Error; err != nil {
			t.Fatalf("set updated_at: %v", err)
		}
	}

	mk(channel, ada, "oldest", base.Add(-3*time.Hour))
	mk(channel, grace, "middle", base.Add(-2*time.Hour))
	mk(channel, ada, "newest", base.Add(-1*time.Hour))
	mk(other, grace, "other-channel", base)

	t.Cleanup(func() {
		db.Where("org_id = ?", org.ID).Delete(&model.Session{})
		db.Where("id IN ?", []uuid.UUID{channel.ID, other.ID}).Delete(&model.Channel{})
		db.Where("id IN ?", []uuid.UUID{ada.ID, grace.ID}).Delete(&model.Agent{})
		db.Where("id = ?", team.ID).Delete(&model.Team{})
		db.Where("id = ?", org.ID).Delete(&model.Org{})
	})

	got, err := RecentChannelSessions(context.Background(), db, org.ID, channel.ID)
	if err != nil {
		t.Fatalf("recent sessions: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 sessions for the channel, got %d (%+v)", len(got), got)
	}
	if got[0].Name != "newest" || got[1].Name != "middle" || got[2].Name != "oldest" {
		t.Fatalf("wrong order: %+v", got)
	}
	if got[0].AgentName != ada.Name {
		t.Fatalf("expected newest handled by %s, got %s", ada.Name, got[0].AgentName)
	}
}

func TestCandidatesFromAgentsUsesDescriptionThenInstructions(t *testing.T) {
	desc := "backend engineer"
	instr := "You are a data analyst who writes SQL."
	empty := ""
	agents := []model.Agent{
		{ID: uuid.New(), Name: "Ada", Description: &desc},
		{ID: uuid.New(), Name: "Grace", Description: &empty, Instructions: &instr},
		{ID: uuid.New(), Name: "Nyx"},
	}

	got := CandidatesFromAgents(agents)
	if len(got) != 3 {
		t.Fatalf("expected 3 candidates, got %d", len(got))
	}
	if got[0].Role != desc {
		t.Fatalf("expected description role, got %q", got[0].Role)
	}
	if got[1].Role != instr {
		t.Fatalf("expected instructions fallback role, got %q", got[1].Role)
	}
	if got[2].Role != "" {
		t.Fatalf("expected empty role, got %q", got[2].Role)
	}
}
