package tasks

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

// A trigger whose configured channel belongs to a team the agent is not in is
// rejected at session creation (hard enforcement).
func TestFindOrCreateTriggerSessionRejectsUnassignedChannel(t *testing.T) {
	db := connectTestDB(t)
	org, agent, _ := seedTriggerSessionFixture(t, db)
	// A team the (team-less) fixture agent does not belong to.
	team := model.Team{OrgID: org.ID, Name: "t-" + uuid.NewString()[:8]}
	if err := db.Create(&team).Error; err != nil {
		t.Fatalf("create team: %v", err)
	}
	// A standard, team-scoped channel whose team does not own the agent.
	channel := model.Channel{
		OrgID:          org.ID,
		Name:           "unassigned-" + uuid.NewString()[:8],
		Kind:           "standard",
		Visibility:     "public",
		TeamID:         &team.ID,
		DefaultAgentID: agent.ID,
		Origin:         "native",
	}
	if err := db.Create(&channel).Error; err != nil {
		t.Fatalf("create channel: %v", err)
	}
	trigger := seedTriggerForSession(t, db, org.ID, agent.ID, &channel.ID)
	handler := &AgentTriggerDispatchHandler{db: db}

	_, err := handler.findOrCreateTriggerSession(context.Background(), &agent, trigger, "github/acme/repo/issues/1")
	if err == nil || !strings.Contains(err.Error(), "does not belong to this channel's team") {
		t.Fatalf("err = %v, want team-mismatch rejection", err)
	}
}

// External/team-less channels (team_id NULL) are exempt: a trigger configured
// with such a channel runs any org agent there without a team-membership check.
// (There is no #system fallback anymore — the channel must be explicit.)
func TestFindOrCreateTriggerSessionTeamlessChannelExempt(t *testing.T) {
	db := connectTestDB(t)
	org, agent, _ := seedTriggerSessionFixture(t, db)
	channel := model.Channel{
		OrgID:               org.ID,
		Name:                "external-" + uuid.NewString()[:8],
		Kind:                "standard",
		Visibility:          "public",
		DefaultAgentID:      agent.ID,
		Origin:              "slack",
		ExternalResourceKey: "C" + uuid.NewString()[:8],
	}
	if err := db.Create(&channel).Error; err != nil {
		t.Fatalf("create team-less channel: %v", err)
	}
	trigger := seedTriggerForSession(t, db, org.ID, agent.ID, &channel.ID)
	handler := &AgentTriggerDispatchHandler{db: db}

	session, err := handler.findOrCreateTriggerSession(context.Background(), &agent, trigger, "github/acme/repo/issues/2")
	if err != nil {
		t.Fatalf("team-less channel trigger session: %v", err)
	}
	if session.ChannelID != channel.ID {
		t.Fatalf("session channel = %s, want %s", session.ChannelID, channel.ID)
	}
}
