package tasks

import (
	"context"
	"strings"
	"testing"

	"github.com/usehivy/hivy/internal/model"
)

// ensureRunSession refuses to open a scheduled session when the agent is not
// assigned to the schedule's (non-system) channel.
func TestScheduleDeliverRejectsUnassignedChannel(t *testing.T) {
	db := connectTestDB(t)
	fx := seedScheduleRunFixture(t, db)
	// Drop the assignment the fixture created so the agent is no longer assigned.
	if err := db.Where("channel_id = ? AND agent_id = ?", fx.channelID, fx.agent.ID).
		Delete(&model.ChannelAgent{}).Error; err != nil {
		t.Fatalf("remove assignment: %v", err)
	}
	handler := &AgentScheduleDeliverHandler{db: db, enqueuer: &fakeTaskEnqueuer{}}

	task, _, err := NewAgentScheduleDeliverTask(AgentScheduleDeliverPayload{RunID: fx.run.ID})
	if err != nil {
		t.Fatalf("new task: %v", err)
	}
	err = handler.Handle(context.Background(), task)
	if err == nil || !strings.Contains(err.Error(), "not assigned to this channel") {
		t.Fatalf("handle err = %v, want not-assigned rejection", err)
	}
}
