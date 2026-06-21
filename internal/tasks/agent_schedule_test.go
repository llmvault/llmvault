package tasks

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/agentschedule"
	"github.com/usehivy/hivy/internal/model"
)

func TestNextScheduleRunAfterFastForwardsStaleInterval(t *testing.T) {
	interval := int64(10)
	base := time.Date(2026, 6, 18, 10, 0, 0, 0, time.UTC)
	now := base.Add(95 * time.Second)
	schedule := model.AgentSchedule{
		ScheduleKind:    agentschedule.KindInterval,
		IntervalSeconds: &interval,
		NextRunAt:       &base,
	}

	next, err := nextScheduleRunAfter(schedule, now)
	if err != nil {
		t.Fatalf("next run: %v", err)
	}
	want := base.Add(100 * time.Second)
	if !next.Equal(want) {
		t.Fatalf("next run = %s, want %s", next, want)
	}
}

func TestScheduledMessagePayloadCarriesBackendScheduleMetadata(t *testing.T) {
	scheduleID := uuid.New()
	runID := uuid.New()
	startedAt := time.Date(2026, 6, 18, 10, 0, 5, 0, time.UTC)
	scheduledAt := time.Date(2026, 6, 18, 10, 0, 0, 0, time.UTC)
	run := model.AgentScheduleRun{
		ID:           runID,
		ScheduleID:   scheduleID,
		RuntimeJobID: "cron-e2e",
		RunKey:       "cron-e2e:2026-06-18T10:00:00Z",
		ScheduledAt:  &scheduledAt,
		Schedule: model.AgentSchedule{
			TaskPrompt: "ship the digest",
		},
	}

	payload := scheduledMessagePayload(run, startedAt)

	if payload["text"] != "ship the digest" {
		t.Fatalf("text = %v", payload["text"])
	}
	if payload["source"] != "cron" {
		t.Fatalf("source = %v", payload["source"])
	}
	if payload["job_id"] != "cron-e2e" {
		t.Fatalf("job_id = %v", payload["job_id"])
	}
	if payload["schedule_id"] != scheduleID.String() {
		t.Fatalf("schedule_id = %v", payload["schedule_id"])
	}
	if payload["schedule_run_id"] != runID.String() {
		t.Fatalf("schedule_run_id = %v", payload["schedule_run_id"])
	}
	if payload["schedule_run_key"] != run.RunKey {
		t.Fatalf("schedule_run_key = %v", payload["schedule_run_key"])
	}
	if payload["schedule_scheduled_at"] != scheduledAt.Format(time.RFC3339) {
		t.Fatalf("schedule_scheduled_at = %v", payload["schedule_scheduled_at"])
	}
	if payload["schedule_started_at"] != startedAt.Format(time.RFC3339) {
		t.Fatalf("schedule_started_at = %v", payload["schedule_started_at"])
	}
	deprecatedWakeKey := "schedule_is_" + "wake"
	if _, ok := payload[deprecatedWakeKey]; ok {
		t.Fatalf("%s should not be present", deprecatedWakeKey)
	}
}
