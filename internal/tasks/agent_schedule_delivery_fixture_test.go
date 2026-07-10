package tasks

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/agentschedule"
	"github.com/usehivy/hivy/internal/model"
)

type scheduleRunFixture struct {
	org       model.Org
	agent     model.Agent
	channelID uuid.UUID
	schedule  model.AgentSchedule
	run       model.AgentScheduleRun
}

func seedScheduleRunFixture(t *testing.T, db *gorm.DB) scheduleRunFixture {
	t.Helper()

	org := model.Org{Name: "schedule-org-" + uuid.NewString()[:8], Active: true}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}

	team := model.Team{OrgID: org.ID, Name: "schedule-team-" + uuid.NewString()[:8]}
	if err := db.Create(&team).Error; err != nil {
		t.Fatalf("create team: %v", err)
	}

	agent := model.Agent{
		OrgID:         &org.ID,
		TeamID:        team.ID,
		Name:          "Agent-" + uuid.NewString()[:8],
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

	channel := model.Channel{
		OrgID:          org.ID,
		TeamID:         team.ID,
		Name:           "schedule-" + uuid.NewString()[:8],
		Kind:           "standard",
		Visibility:     "public",
		DefaultAgentID: agent.ID,
		Origin:         "native",
	}
	if err := db.Create(&channel).Error; err != nil {
		t.Fatalf("create channel: %v", err)
	}

	schedule := model.AgentSchedule{
		OrgID:        org.ID,
		AgentID:      agent.ID,
		RuntimeJobID: "cron-" + uuid.NewString()[:8],
		ScheduleKind: agentschedule.KindCron,
		Channel:      channel.ID.String(),
		Description:  "Nightly digest",
		TaskPrompt:   "ship the nightly digest",
		Status:       "active",
	}
	if err := db.Create(&schedule).Error; err != nil {
		t.Fatalf("create schedule: %v", err)
	}

	scheduledAt := time.Now().UTC()
	run := model.AgentScheduleRun{
		OrgID:        org.ID,
		AgentID:      agent.ID,
		ScheduleID:   schedule.ID,
		RuntimeJobID: schedule.RuntimeJobID,
		RunKey:       schedule.RuntimeJobID + ":" + scheduledAt.Format(time.RFC3339),
		Status:       agentschedule.RunStatusQueued,
		ScheduledAt:  &scheduledAt,
	}
	if err := db.Create(&run).Error; err != nil {
		t.Fatalf("create run: %v", err)
	}

	t.Cleanup(func() {
		db.Where("schedule_id = ?", schedule.ID).Delete(&model.AgentScheduleRun{})
		var sessionIDs []uuid.UUID
		db.Model(&model.Session{}).Where("org_id = ?", org.ID).Pluck("id", &sessionIDs)
		if len(sessionIDs) > 0 {
			db.Where("session_id IN ?", sessionIDs).Delete(&model.SessionMessageQueue{})
			db.Where("session_id IN ?", sessionIDs).Delete(&model.SessionEvent{})
			db.Where("id IN ?", sessionIDs).Delete(&model.Session{})
		}
		db.Where("id = ?", schedule.ID).Delete(&model.AgentSchedule{})
		db.Where("id = ?", channel.ID).Delete(&model.Channel{})
		db.Where("id = ?", agent.ID).Delete(&model.Agent{})
		db.Where("id = ?", org.ID).Delete(&model.Org{})
	})

	return scheduleRunFixture{org: org, agent: agent, channelID: channel.ID, schedule: schedule, run: run}
}
