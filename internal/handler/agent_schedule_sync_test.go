package handler

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

func TestIntegration_AgentScheduleEvents_UpdateSchedulesAndRuns(t *testing.T) {
	db := connectAgentSkillSyncTestDB(t)
	org := model.Org{Name: "schedule-sync-" + uuid.NewString()}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	agent := model.Agent{OrgID: &org.ID, Name: "Aria", Model: "test"}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	sb := agentScheduleTestSandbox(t, db, org.ID, agent.ID)
	t.Cleanup(func() { db.Where("id = ?", org.ID).Delete(&model.Org{}) })

	h := NewAgentOutboundWebhookHandler(db, nil, nil)
	now := time.Date(2026, 5, 13, 10, 0, 0, 0, time.UTC)
	payload := agentScheduleTestPayload(now)
	h.storeAndMaybeEnqueue(t.Context(), &sb, outboundTestEvent(t, "schedule.created", now, payload))
	h.storeAndMaybeEnqueue(t.Context(), &sb, outboundTestEvent(t, "schedule.paused", now.Add(time.Minute), payload))
	payload["state"] = "active"
	payload["run_key"] = "cron-1:" + now.Add(time.Hour).Format(time.RFC3339)
	payload["scheduled_at"] = now.Add(time.Hour).Format(time.RFC3339)
	payload["started_at"] = now.Add(time.Hour).Format(time.RFC3339)
	h.storeAndMaybeEnqueue(t.Context(), &sb, outboundTestEvent(t, "schedule.run_started", now.Add(time.Hour), payload))
	payload["repeat_completed"] = float64(1)
	payload["last_status"] = "completed"
	payload["completed_at"] = now.Add(time.Hour + time.Minute).Format(time.RFC3339)
	payload["duration_ms"] = float64(60000)
	h.storeAndMaybeEnqueue(t.Context(), &sb, outboundTestEvent(t, "schedule.run_completed", now.Add(time.Hour+time.Minute), payload))
	h.storeAndMaybeEnqueue(t.Context(), &sb, outboundTestEvent(t, "schedule.cancelled", now.Add(2*time.Hour), payload))
	h.storeAndMaybeEnqueue(t.Context(), &sb, outboundTestEvent(t, "schedule.cancelled", now.Add(2*time.Hour), payload))

	assertAgentScheduleMirror(t, db, agent.ID, payload)

	agent2 := model.Agent{OrgID: &org.ID, Name: "Mira", Model: "test"}
	if err := db.Create(&agent2).Error; err != nil {
		t.Fatalf("create agent2: %v", err)
	}
	sb2 := agentScheduleTestSandbox(t, db, org.ID, agent2.ID)
	h.storeAndMaybeEnqueue(t.Context(), &sb2, outboundTestEvent(t, "schedule.created", now, payload))
	var scheduleCount int64
	db.Model(&model.AgentSchedule{}).Where("runtime_job_id = ?", "cron-1").Count(&scheduleCount)
	if scheduleCount != 2 {
		t.Fatalf("same runtime job id for different agents should create separate schedules, got %d", scheduleCount)
	}
}

func TestIntegration_AgentScheduleMalformedPayload_StoresEventOnly(t *testing.T) {
	db := connectAgentSkillSyncTestDB(t)
	org, agent, sb := agentScheduleTestScope(t, db, "schedule-malformed-")
	t.Cleanup(func() { db.Where("id = ?", org.ID).Delete(&model.Org{}) })

	h := NewAgentOutboundWebhookHandler(db, nil, nil)
	h.storeAndMaybeEnqueue(t.Context(), &sb, outboundTestEvent(t, "schedule.created", time.Now().UTC(), map[string]any{"session_id": "C123-456.789"}))

	assertScheduleEventWithoutMirror(t, db, agent.ID)
}

func TestIntegration_AgentScheduleNonCronSource_StoresEventOnly(t *testing.T) {
	db := connectAgentSkillSyncTestDB(t)
	org, agent, sb := agentScheduleTestScope(t, db, "schedule-non-cron-")
	t.Cleanup(func() { db.Where("id = ?", org.ID).Delete(&model.Org{}) })

	h := NewAgentOutboundWebhookHandler(db, nil, nil)
	h.storeAndMaybeEnqueue(t.Context(), &sb, outboundTestEvent(t, "schedule.created", time.Now().UTC(), map[string]any{
		"session_id": "C123-456.789",
		"job_id":     "delegate-1",
		"source":     "delegate",
	}))

	assertScheduleEventWithoutMirror(t, db, agent.ID)
}

func agentScheduleTestScope(t *testing.T, db interface {
	Create(value any) *gorm.DB
}, orgPrefix string) (model.Org, model.Agent, model.Sandbox) {
	t.Helper()
	org := model.Org{Name: orgPrefix + uuid.NewString()}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	agent := model.Agent{OrgID: &org.ID, Name: "Aria", Model: "test"}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	return org, agent, agentScheduleTestSandbox(t, db, org.ID, agent.ID)
}

func agentScheduleTestSandbox(t *testing.T, db interface {
	Create(value any) *gorm.DB
}, orgID uuid.UUID, agentID uuid.UUID) model.Sandbox {
	t.Helper()
	sb := model.Sandbox{
		OrgID:                  &orgID,
		AgentID:                &agentID,
		ExternalID:             "sandbox-" + uuid.NewString(),
		RuntimeURL:             "https://runtime.test",
		EncryptedRuntimeSecret: []byte("secret"),
		Status:                 "running",
	}
	if err := db.Create(&sb).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	return sb
}

func agentScheduleTestPayload(now time.Time) map[string]any {
	return map[string]any{
		"session_id":         "C123-456.789",
		"source":             "cron",
		"job_id":             "cron-1",
		"state":              "active",
		"channel":            "C123",
		"description":        "Deploy health",
		"task_prompt":        "Check deploy health",
		"interval_seconds":   float64(3600),
		"repeat_count":       float64(5),
		"repeat_completed":   float64(0),
		"next_run_at":        now.Add(time.Hour).Format(time.RFC3339),
		"created_by_session": "C123-456.789",
		"created_at":         now.Format(time.RFC3339),
	}
}

func assertAgentScheduleMirror(t *testing.T, db *gorm.DB, agentID uuid.UUID, payload map[string]any) {
	t.Helper()
	var eventCount int64
	db.Model(&model.AgentSessionEvent{}).Where("agent_id = ? AND event_type LIKE ?", agentID, "schedule.%").Count(&eventCount)
	// The two byte-identical schedule.cancelled events are a redelivery and dedupe
	// to one row via the (sandbox_id, event_id) idempotency key.
	if eventCount != 5 {
		t.Fatalf("schedule event count = %d", eventCount)
	}
	var schedule model.AgentSchedule
	if err := db.Where("agent_id = ? AND runtime_job_id = ?", agentID, "cron-1").First(&schedule).Error; err != nil {
		t.Fatalf("load schedule: %v", err)
	}
	if schedule.Status != "cancelled" || schedule.CancelledAt == nil {
		t.Fatalf("schedule final state = %#v", schedule)
	}
	if schedule.RepeatCompleted != 1 || schedule.LastStatus != "completed" {
		t.Fatalf("schedule run fields = %#v", schedule)
	}
	var scheduleCount int64
	db.Model(&model.AgentSchedule{}).Where("agent_id = ? AND runtime_job_id = ?", agentID, "cron-1").Count(&scheduleCount)
	if scheduleCount != 1 {
		t.Fatalf("schedule count = %d", scheduleCount)
	}
	var run model.AgentScheduleRun
	if err := db.Where("schedule_id = ? AND run_key = ?", schedule.ID, payload["run_key"]).First(&run).Error; err != nil {
		t.Fatalf("load run: %v", err)
	}
	if run.Status != "completed" || run.DurationMS == nil || *run.DurationMS != 60000 {
		t.Fatalf("run = %#v", run)
	}
	var runCount int64
	db.Model(&model.AgentScheduleRun{}).Where("schedule_id = ?", schedule.ID).Count(&runCount)
	if runCount != 1 {
		t.Fatalf("run count = %d", runCount)
	}
}

func assertScheduleEventWithoutMirror(t *testing.T, db *gorm.DB, agentID uuid.UUID) {
	t.Helper()
	var eventCount int64
	db.Model(&model.AgentSessionEvent{}).Where("agent_id = ? AND event_type = ?", agentID, "schedule.created").Count(&eventCount)
	if eventCount != 1 {
		t.Fatalf("event count = %d", eventCount)
	}
	var scheduleCount int64
	db.Model(&model.AgentSchedule{}).Where("agent_id = ?", agentID).Count(&scheduleCount)
	if scheduleCount != 0 {
		t.Fatalf("schedule count = %d", scheduleCount)
	}
}

func outboundTestEvent(t *testing.T, eventType string, at time.Time, payload map[string]any) *agentOutboundEvent {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return &agentOutboundEvent{EventType: eventType, Payload: body, At: at}
}
