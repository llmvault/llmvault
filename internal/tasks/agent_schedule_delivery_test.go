package tasks

import (
	"context"
	"testing"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/runtimeevents"
)

// A run that has to create a new session must persist the scheduled prompt to
// the transcript (so the UI and the naming task can see it), link the queue row
// to that event, and enqueue the auto-naming task.
func TestScheduleDeliverNewSessionPersistsEventAndEnqueuesNaming(t *testing.T) {
	db := connectTestDB(t)
	fx := seedScheduleRunFixture(t, db)
	enq := &fakeTaskEnqueuer{}
	handler := &AgentScheduleDeliverHandler{db: db, enqueuer: enq}

	task, _, err := NewAgentScheduleDeliverTask(AgentScheduleDeliverPayload{RunID: fx.run.ID})
	if err != nil {
		t.Fatalf("new task: %v", err)
	}
	if err := handler.Handle(context.Background(), task); err != nil {
		t.Fatalf("handle: %v", err)
	}

	// The run now points at a freshly created session.
	var run model.AgentScheduleRun
	if err := db.First(&run, "id = ?", fx.run.ID).Error; err != nil {
		t.Fatalf("reload run: %v", err)
	}
	if run.SessionID == nil {
		t.Fatal("expected run.session_id to be set")
	}
	sessionID := *run.SessionID

	// The scheduled prompt is on the transcript as a user.message.received event.
	wantEventID := "schedule:" + fx.run.RunKey
	var event model.SessionEvent
	if err := db.Where("session_id = ? AND event_id = ?", sessionID, wantEventID).First(&event).Error; err != nil {
		t.Fatalf("load schedule session event: %v", err)
	}
	if event.EventType != runtimeevents.EventUserMessageReceived {
		t.Fatalf("event_type = %q, want %q", event.EventType, runtimeevents.EventUserMessageReceived)
	}
	if event.Source != scheduleSource {
		t.Fatalf("source = %q, want %q", event.Source, scheduleSource)
	}
	if event.Payload["text"] != fx.schedule.TaskPrompt {
		t.Fatalf("payload text = %v, want %q", event.Payload["text"], fx.schedule.TaskPrompt)
	}

	// The queue row links to the transcript event.
	var queue model.SessionMessageQueue
	if err := db.Where("session_id = ?", sessionID).First(&queue).Error; err != nil {
		t.Fatalf("load queue: %v", err)
	}
	if queue.SessionEventID == nil || *queue.SessionEventID != event.ID {
		t.Fatalf("queue.session_event_id = %v, want %v", queue.SessionEventID, event.ID)
	}

	// Auto-naming was enqueued for the new session.
	if n := countTasksOfType(enq, TypeSessionName); n != 1 {
		t.Fatalf("session name enqueues = %d, want 1", n)
	}
}

// A run that reuses an existing session must not re-enqueue naming and must not
// write a duplicate transcript event.
func TestScheduleDeliverExistingSessionSkipsNaming(t *testing.T) {
	db := connectTestDB(t)
	fx := seedScheduleRunFixture(t, db)

	existing := model.Session{
		OrgID:             fx.org.ID,
		ChannelID:         fx.channelID,
		AgentID:           fx.agent.ID,
		Model:             fx.agent.Model,
		ReasoningEffort:   "high",
		Source:            scheduleSource,
		SourceResourceKey: fx.run.RunKey,
		Name:              "existing-schedule-session",
		Status:            "active",
		AgentTurnStatus:   model.SessionAgentTurnIdle,
		IntegrationScopes: model.JSON{},
	}
	if err := db.Create(&existing).Error; err != nil {
		t.Fatalf("create existing session: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", existing.ID).Delete(&model.Session{}) })
	if err := db.Model(&model.AgentScheduleRun{}).Where("id = ?", fx.run.ID).Update("session_id", existing.ID).Error; err != nil {
		t.Fatalf("attach session: %v", err)
	}

	enq := &fakeTaskEnqueuer{}
	handler := &AgentScheduleDeliverHandler{db: db, enqueuer: enq}
	task, _, err := NewAgentScheduleDeliverTask(AgentScheduleDeliverPayload{RunID: fx.run.ID})
	if err != nil {
		t.Fatalf("new task: %v", err)
	}
	if err := handler.Handle(context.Background(), task); err != nil {
		t.Fatalf("handle: %v", err)
	}

	if n := countTasksOfType(enq, TypeSessionName); n != 0 {
		t.Fatalf("session name enqueues = %d, want 0 for existing session", n)
	}
	var events int64
	if err := db.Model(&model.SessionEvent{}).Where("session_id = ?", existing.ID).Count(&events).Error; err != nil {
		t.Fatalf("count events: %v", err)
	}
	if events != 0 {
		t.Fatalf("events on reused session = %d, want 0", events)
	}
}

// Running the delivery twice for the same run key must not duplicate the
// transcript event: the second run reuses the session created by the first.
func TestScheduleDeliverIsIdempotentAcrossRuns(t *testing.T) {
	db := connectTestDB(t)
	fx := seedScheduleRunFixture(t, db)
	enq := &fakeTaskEnqueuer{}
	handler := &AgentScheduleDeliverHandler{db: db, enqueuer: enq}
	task, _, err := NewAgentScheduleDeliverTask(AgentScheduleDeliverPayload{RunID: fx.run.ID})
	if err != nil {
		t.Fatalf("new task: %v", err)
	}
	if err := handler.Handle(context.Background(), task); err != nil {
		t.Fatalf("first handle: %v", err)
	}
	if err := handler.Handle(context.Background(), task); err != nil {
		t.Fatalf("second handle: %v", err)
	}

	var run model.AgentScheduleRun
	if err := db.First(&run, "id = ?", fx.run.ID).Error; err != nil {
		t.Fatalf("reload run: %v", err)
	}
	if run.SessionID == nil {
		t.Fatal("expected run.session_id to be set")
	}
	wantEventID := "schedule:" + fx.run.RunKey
	var events int64
	if err := db.Model(&model.SessionEvent{}).
		Where("session_id = ? AND event_id = ?", *run.SessionID, wantEventID).
		Count(&events).Error; err != nil {
		t.Fatalf("count events: %v", err)
	}
	if events != 1 {
		t.Fatalf("schedule events = %d, want 1", events)
	}
	// Naming enqueued only on the first (session-creating) run.
	if n := countTasksOfType(enq, TypeSessionName); n != 1 {
		t.Fatalf("session name enqueues = %d, want 1", n)
	}
}

// A scheduled session must inherit the agent's configured reasoning effort, and
// fall back to "high" only when the agent leaves it unset.
func TestScheduleDeliverNewSessionHonorsAgentReasoningEffort(t *testing.T) {
	db := connectTestDB(t)

	deliver := func(t *testing.T, fx scheduleRunFixture) model.Session {
		t.Helper()
		enq := &fakeTaskEnqueuer{}
		handler := &AgentScheduleDeliverHandler{db: db, enqueuer: enq}
		task, _, err := NewAgentScheduleDeliverTask(AgentScheduleDeliverPayload{RunID: fx.run.ID})
		if err != nil {
			t.Fatalf("new task: %v", err)
		}
		if err := handler.Handle(context.Background(), task); err != nil {
			t.Fatalf("handle: %v", err)
		}
		var run model.AgentScheduleRun
		if err := db.First(&run, "id = ?", fx.run.ID).Error; err != nil {
			t.Fatalf("reload run: %v", err)
		}
		if run.SessionID == nil {
			t.Fatal("expected run.session_id to be set")
		}
		var session model.Session
		if err := db.First(&session, "id = ?", *run.SessionID).Error; err != nil {
			t.Fatalf("load session: %v", err)
		}
		return session
	}

	t.Run("honors configured effort", func(t *testing.T) {
		fx := seedScheduleRunFixture(t, db)
		if err := db.Model(&model.Agent{}).Where("id = ?", fx.agent.ID).
			Update("default_reasoning_effort", "medium").Error; err != nil {
			t.Fatalf("set agent effort: %v", err)
		}
		if got := deliver(t, fx).ReasoningEffort; got != "medium" {
			t.Fatalf("reasoning_effort = %q, want medium", got)
		}
	})

	t.Run("falls back to backend default (low) when unset", func(t *testing.T) {
		fx := seedScheduleRunFixture(t, db)
		if got := deliver(t, fx).ReasoningEffort; got != model.DefaultReasoningEffort {
			t.Fatalf("reasoning_effort = %q, want %q", got, model.DefaultReasoningEffort)
		}
	})
}
