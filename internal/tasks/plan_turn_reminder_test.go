package tasks

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/runtimeevents"
)

func planReminderTestSession(t *testing.T, db *gorm.DB) model.Session {
	t.Helper()
	org, agent, channel := seedSessionRuntimeSelectionFixture(t, db, "plan-reminder")
	return seedSessionRuntimeSelectionSession(t, db, org.ID, channel.ID, agent.ID, nil)
}

func seedPlanEvent(t *testing.T, db *gorm.DB, session model.Session, seq int64, items ...map[string]any) {
	t.Helper()
	plan := make([]any, 0, len(items))
	for _, item := range items {
		plan = append(plan, item)
	}
	event := model.SessionEvent{
		OrgID:          session.OrgID,
		SessionID:      session.ID,
		AgentID:        session.AgentID,
		EventID:        "evt_plan_" + uuid.NewString()[:8],
		EventType:      runtimeevents.EventPlanUpdated,
		Source:         "runtime",
		SequenceNumber: seq,
		Durability:     "durable",
		Payload:        model.JSON{"plan": plan},
		EventAt:        time.Now().UTC(),
	}
	if err := db.Create(&event).Error; err != nil {
		t.Fatalf("create plan event: %v", err)
	}
}

func planStep(step, status string) map[string]any {
	return map[string]any{"step": step, "status": status}
}

func reminderEvents(t *testing.T, db *gorm.DB, sessionID uuid.UUID) []model.SessionEvent {
	t.Helper()
	var events []model.SessionEvent
	if err := db.Where("session_id = ? AND source = ?", sessionID, planReminderSource).
		Find(&events).Error; err != nil {
		t.Fatalf("load reminder events: %v", err)
	}
	return events
}

func reminderQueueRows(t *testing.T, db *gorm.DB, sessionID uuid.UUID) []model.SessionMessageQueue {
	t.Helper()
	var rows []model.SessionMessageQueue
	if err := db.Where("session_id = ?", sessionID).Order("sequence_number").Find(&rows).Error; err != nil {
		t.Fatalf("load queue rows: %v", err)
	}
	return rows
}

func TestPlanTurnReminderCreatesReminderForIncompletePlan(t *testing.T) {
	db := connectTestDB(t)
	session := planReminderTestSession(t, db)
	seedPlanEvent(t, db, session, 1,
		planStep("Explore the code", "completed"),
		planStep("Write the fix", "in_progress"),
		planStep("Add tests", "pending"),
	)

	enq := &fakeTaskEnqueuer{}
	handler := NewPlanTurnReminderHandler(db, enq)
	if err := handler.remind(t.Context(), PlanTurnReminderPayload{SessionID: session.ID, TurnID: "turn-1"}); err != nil {
		t.Fatalf("remind: %v", err)
	}

	events := reminderEvents(t, db, session.ID)
	if len(events) != 1 {
		t.Fatalf("reminder events = %d, want 1", len(events))
	}
	got := events[0]
	wantID := planReminderEventIDPrefix + session.ID.String() + ":turn-1"
	if got.EventID != wantID {
		t.Fatalf("reminder event id = %q, want %q", got.EventID, wantID)
	}
	if got.EventType != runtimeevents.EventUserMessageReceived {
		t.Fatalf("reminder event type = %q, want %q", got.EventType, runtimeevents.EventUserMessageReceived)
	}
	if got.Source != planReminderSource {
		t.Fatalf("reminder source = %q, want %q", got.Source, planReminderSource)
	}
	text, _ := got.Payload["text"].(string)
	if !strings.Contains(text, "Write the fix") || !strings.Contains(text, "Add tests") {
		t.Fatalf("reminder text missing incomplete steps: %q", text)
	}
	if strings.Contains(text, "Explore the code") {
		t.Fatalf("reminder text should omit completed steps: %q", text)
	}

	rows := reminderQueueRows(t, db, session.ID)
	if len(rows) != 1 {
		t.Fatalf("queue rows = %d, want 1", len(rows))
	}
	if rows[0].SessionEventID == nil || *rows[0].SessionEventID != got.ID {
		t.Fatalf("queue row not linked to reminder event")
	}
	if n := countTasksOfType(enq, TypeSessionMessageDeliver); n != 1 {
		t.Fatalf("session delivery tasks = %d, want 1", n)
	}

	// Redelivery of the same completed turn must not create a second reminder
	// (the pending reminder row is caught by the pending-queue guard, and the
	// stable event id dedupes the transcript row).
	if err := handler.remind(t.Context(), PlanTurnReminderPayload{SessionID: session.ID, TurnID: "turn-1"}); err != nil {
		t.Fatalf("remind (redelivery): %v", err)
	}
	if events := reminderEvents(t, db, session.ID); len(events) != 1 {
		t.Fatalf("reminder events after redelivery = %d, want 1", len(events))
	}
	if rows := reminderQueueRows(t, db, session.ID); len(rows) != 1 {
		t.Fatalf("queue rows after redelivery = %d, want 1", len(rows))
	}
}

func TestPlanTurnReminderSkipsSecondConsecutiveReminder(t *testing.T) {
	db := connectTestDB(t)
	session := planReminderTestSession(t, db)
	seedPlanEvent(t, db, session, 1, planStep("Ship it", "in_progress"))

	enq := &fakeTaskEnqueuer{}
	handler := NewPlanTurnReminderHandler(db, enq)
	if err := handler.remind(t.Context(), PlanTurnReminderPayload{SessionID: session.ID, TurnID: "turn-1"}); err != nil {
		t.Fatalf("first remind: %v", err)
	}
	if rows := reminderQueueRows(t, db, session.ID); len(rows) != 1 {
		t.Fatalf("queue rows after first remind = %d, want 1", len(rows))
	}

	// Simulate the reminder being delivered and the follow-up turn completing
	// again without the agent touching the plan: mark the reminder delivered and
	// return the session to idle.
	if err := db.Model(&model.SessionMessageQueue{}).
		Where("session_id = ?", session.ID).
		Update("status", "delivered").Error; err != nil {
		t.Fatalf("mark reminder delivered: %v", err)
	}

	if err := handler.remind(t.Context(), PlanTurnReminderPayload{SessionID: session.ID, TurnID: "turn-2"}); err != nil {
		t.Fatalf("second remind: %v", err)
	}
	if events := reminderEvents(t, db, session.ID); len(events) != 1 {
		t.Fatalf("reminder events after second remind = %d, want 1 (loop guard)", len(events))
	}
	if rows := reminderQueueRows(t, db, session.ID); len(rows) != 1 {
		t.Fatalf("queue rows after second remind = %d, want 1 (loop guard)", len(rows))
	}

	// A non-reminder message delivered in between clears the loop guard, so a new
	// reminder is allowed after the next completion.
	seedDeliveredUserMessage(t, db, session)
	if err := handler.remind(t.Context(), PlanTurnReminderPayload{SessionID: session.ID, TurnID: "turn-3"}); err != nil {
		t.Fatalf("third remind: %v", err)
	}
	if events := reminderEvents(t, db, session.ID); len(events) != 2 {
		t.Fatalf("reminder events after intervening message = %d, want 2", len(events))
	}
}

func seedDeliveredUserMessage(t *testing.T, db *gorm.DB, session model.Session) {
	t.Helper()
	event := model.SessionEvent{
		OrgID:          session.OrgID,
		SessionID:      session.ID,
		AgentID:        session.AgentID,
		EventID:        "user-" + uuid.NewString()[:8],
		EventType:      runtimeevents.EventUserMessageReceived,
		Source:         "web",
		SequenceNumber: 0,
		Durability:     "durable",
		Payload:        model.JSON{"text": "one more thing"},
		EventAt:        time.Now().UTC(),
	}
	if err := db.Create(&event).Error; err != nil {
		t.Fatalf("create user message event: %v", err)
	}
	seq, err := nextSessionQueueSequence(db, session.ID)
	if err != nil {
		t.Fatalf("next queue sequence: %v", err)
	}
	eventID := event.ID
	queue := model.SessionMessageQueue{
		OrgID:          session.OrgID,
		SessionID:      session.ID,
		SessionEventID: &eventID,
		MessageText:    "one more thing",
		MessagePayload: model.JSON{"text": "one more thing"},
		SequenceNumber: seq,
		Status:         "delivered",
	}
	if err := db.Create(&queue).Error; err != nil {
		t.Fatalf("create user message queue row: %v", err)
	}
}

func TestPlanTurnReminderSkipsWhenPlanComplete(t *testing.T) {
	db := connectTestDB(t)
	session := planReminderTestSession(t, db)
	seedPlanEvent(t, db, session, 1,
		planStep("Explore", "completed"),
		planStep("Fix", "completed"),
	)

	enq := &fakeTaskEnqueuer{}
	handler := NewPlanTurnReminderHandler(db, enq)
	if err := handler.remind(t.Context(), PlanTurnReminderPayload{SessionID: session.ID, TurnID: "turn-1"}); err != nil {
		t.Fatalf("remind: %v", err)
	}
	if events := reminderEvents(t, db, session.ID); len(events) != 0 {
		t.Fatalf("reminder events = %d, want 0", len(events))
	}
	if n := countTasksOfType(enq, TypeSessionMessageDeliver); n != 0 {
		t.Fatalf("session delivery tasks = %d, want 0", n)
	}
}

func TestPlanTurnReminderSkipsWhenNoPlan(t *testing.T) {
	db := connectTestDB(t)
	session := planReminderTestSession(t, db)

	enq := &fakeTaskEnqueuer{}
	handler := NewPlanTurnReminderHandler(db, enq)
	if err := handler.remind(t.Context(), PlanTurnReminderPayload{SessionID: session.ID, TurnID: "turn-1"}); err != nil {
		t.Fatalf("remind: %v", err)
	}
	if events := reminderEvents(t, db, session.ID); len(events) != 0 {
		t.Fatalf("reminder events = %d, want 0", len(events))
	}
}

func TestPlanTurnReminderSkipsWhenQueuePending(t *testing.T) {
	db := connectTestDB(t)
	session := planReminderTestSession(t, db)
	seedPlanEvent(t, db, session, 1, planStep("Keep going", "pending"))
	pending := model.SessionMessageQueue{
		OrgID:          session.OrgID,
		SessionID:      session.ID,
		MessageText:    "already queued",
		MessagePayload: model.JSON{"text": "already queued"},
		SequenceNumber: 1,
		Status:         "pending",
	}
	if err := db.Create(&pending).Error; err != nil {
		t.Fatalf("create pending queue row: %v", err)
	}

	enq := &fakeTaskEnqueuer{}
	handler := NewPlanTurnReminderHandler(db, enq)
	if err := handler.remind(t.Context(), PlanTurnReminderPayload{SessionID: session.ID, TurnID: "turn-1"}); err != nil {
		t.Fatalf("remind: %v", err)
	}
	if events := reminderEvents(t, db, session.ID); len(events) != 0 {
		t.Fatalf("reminder events = %d, want 0 (queue pending)", len(events))
	}
}

func TestPlanTurnReminderSkipsWhenTurnActive(t *testing.T) {
	db := connectTestDB(t)
	session := planReminderTestSession(t, db)
	if err := db.Model(&session).Update("agent_turn_status", model.SessionAgentTurnActive).Error; err != nil {
		t.Fatalf("mark turn active: %v", err)
	}
	seedPlanEvent(t, db, session, 1, planStep("Keep going", "pending"))

	enq := &fakeTaskEnqueuer{}
	handler := NewPlanTurnReminderHandler(db, enq)
	if err := handler.remind(t.Context(), PlanTurnReminderPayload{SessionID: session.ID, TurnID: "turn-1"}); err != nil {
		t.Fatalf("remind: %v", err)
	}
	if events := reminderEvents(t, db, session.ID); len(events) != 0 {
		t.Fatalf("reminder events = %d, want 0 (turn active)", len(events))
	}
}
