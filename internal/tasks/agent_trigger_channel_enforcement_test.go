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
	// A team the fixture agent does not belong to.
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
		TeamID:         team.ID,
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
