package tasks

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

// ensureRunSession refuses to open a scheduled session when the agent does not
// belong to the schedule's (team-scoped) channel's team.
func TestScheduleDeliverRejectsUnassignedChannel(t *testing.T) {
	db := connectTestDB(t)
	fx := seedScheduleRunFixture(t, db)
	// Scope the schedule's channel to a team the fixture agent does not belong
	// to, so the agent may no longer act in it.
	team := model.Team{OrgID: fx.org.ID, Name: "t-" + uuid.NewString()[:8]}
	if err := db.Create(&team).Error; err != nil {
		t.Fatalf("create team: %v", err)
	}
	if err := db.Model(&model.Channel{}).Where("id = ?", fx.channelID).
		Update("team_id", team.ID).Error; err != nil {
		t.Fatalf("scope channel to team: %v", err)
	}
	handler := &AgentScheduleDeliverHandler{db: db, enqueuer: &fakeTaskEnqueuer{}}

	task, _, err := NewAgentScheduleDeliverTask(AgentScheduleDeliverPayload{RunID: fx.run.ID})
	if err != nil {
		t.Fatalf("new task: %v", err)
	}
	err = handler.Handle(context.Background(), task)
	if err == nil || !strings.Contains(err.Error(), "does not belong to this channel's team") {
		t.Fatalf("handle err = %v, want team-mismatch rejection", err)
	}
}
