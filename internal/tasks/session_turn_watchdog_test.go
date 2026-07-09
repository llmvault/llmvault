package tasks

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

// watchdogSeedOrg creates the org/agent/channel a session needs.
func watchdogSeedOrg(t *testing.T, db *gorm.DB) (orgID, agentID, channelID uuid.UUID) {
	t.Helper()
	org := model.Org{Name: "turn-wd-" + uuid.NewString()[:8], Active: true}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	team := model.Team{OrgID: org.ID, Name: "turn-wd-team-" + uuid.NewString()[:8]}
	if err := db.Create(&team).Error; err != nil {
		t.Fatalf("create team: %v", err)
	}
	agent := model.Agent{
		OrgID: &org.ID, TeamID: team.ID, Name: "turn-wd-" + uuid.NewString()[:8], Model: "m",
		Tools: model.JSON{}, McpServers: model.RawJSON("[]"), Skills: model.JSON{},
		RuntimeConfig: model.JSON{}, Permissions: model.JSON{}, Resources: model.JSON{}, Status: "active",
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	channel := model.Channel{
		OrgID: org.ID, Name: "turn-wd-" + uuid.NewString()[:8], Kind: "standard",
		Visibility: "public", DefaultAgentID: agent.ID, Origin: "native",
	}
	if err := db.Create(&channel).Error; err != nil {
		t.Fatalf("create channel: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", org.ID).Delete(&model.Org{}) })
	return org.ID, agent.ID, channel.ID
}

func seedTurnSession(t *testing.T, db *gorm.DB, orgID, agentID, chanID uuid.UUID, turn string, startedAt *time.Time, lastEvent *time.Time) uuid.UUID {
	t.Helper()
	sess := model.Session{
		OrgID: orgID, ChannelID: chanID, AgentID: agentID, Model: "m",
		ReasoningEffort: "high", Source: "web", SourceResourceKey: uuid.NewString(),
		Status: "active", AgentTurnStatus: turn, AgentTurnStartedAt: startedAt,
		IntegrationScopes: model.JSON{},
	}
	if err := db.Create(&sess).Error; err != nil {
		t.Fatalf("create session: %v", err)
	}
	if lastEvent != nil {
		ev := model.SessionEvent{
			SessionID: sess.ID, OrgID: orgID, AgentID: agentID,
			EventType: "message", EventAt: *lastEvent,
		}
		if err := db.Create(&ev).Error; err != nil {
			t.Fatalf("create event: %v", err)
		}
	}
	return sess.ID
}

func turnStatusOf(t *testing.T, db *gorm.DB, id uuid.UUID) (string, string) {
	t.Helper()
	var s model.Session
	if err := db.First(&s, "id = ?", id).Error; err != nil {
		t.Fatalf("load session: %v", err)
	}
	return s.AgentTurnStatus, s.AgentTurnLastOutcome
}

func TestSessionTurnWatchdogResetsStuckTurns(t *testing.T) {
	db := connectTestDB(t)
	ctx := context.Background()
	orgID, agentID, chanID := watchdogSeedOrg(t, db)

	old := time.Now().Add(-2 * time.Hour)
	recent := time.Now().Add(-2 * time.Minute)

	// Stuck: active turn opened long ago, last event long ago -> reset.
	stuck := seedTurnSession(t, db, orgID, agentID, chanID, model.SessionAgentTurnActive, &old, &old)
	// Live long turn: opened long ago BUT still emitting events -> left alone.
	live := seedTurnSession(t, db, orgID, agentID, chanID, model.SessionAgentTurnActive, &old, &recent)
	// Fresh turn: just started, no events yet -> left alone.
	fresh := seedTurnSession(t, db, orgID, agentID, chanID, model.SessionAgentTurnActive, &recent, nil)
	// Already idle -> untouched.
	idle := seedTurnSession(t, db, orgID, agentID, chanID, model.SessionAgentTurnIdle, nil, &old)

	if err := NewSessionTurnWatchdogHandler(db).Handle(ctx, nil); err != nil {
		t.Fatalf("watchdog: %v", err)
	}

	if s, out := turnStatusOf(t, db, stuck); s != model.SessionAgentTurnIdle || out != model.SessionAgentTurnOutcomeFailed {
		t.Fatalf("stuck turn = (%q,%q), want (idle,failed)", s, out)
	}
	if s, _ := turnStatusOf(t, db, live); s != model.SessionAgentTurnActive {
		t.Fatalf("live long turn was reset: %q", s)
	}
	if s, _ := turnStatusOf(t, db, fresh); s != model.SessionAgentTurnActive {
		t.Fatalf("fresh turn was reset: %q", s)
	}
	if s, _ := turnStatusOf(t, db, idle); s != model.SessionAgentTurnIdle {
		t.Fatalf("idle turn changed: %q", s)
	}
}
