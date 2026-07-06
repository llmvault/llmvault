package tasks

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/trigger/hivy"
)

func TestSessionReflectionScanEnqueuesOnlyIdleSessionsWithNewEvents(t *testing.T) {
	db := connectTestDB(t)
	now := time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC)
	fx := seedReflectionFixture(t, db, now.Add(-10*time.Minute))
	active := seedReflectionFixture(t, db, now.Add(-10*time.Minute))
	recent := seedReflectionFixture(t, db, now.Add(-time.Minute))
	locked := seedReflectionFixture(t, db, now.Add(-10*time.Minute))
	if err := db.Model(&model.Session{}).Where("id = ?", active.session.ID).
		Update("agent_turn_status", model.SessionAgentTurnActive).Error; err != nil {
		t.Fatalf("mark active: %v", err)
	}
	if err := db.Create(&model.SessionReflectionState{
		SessionID:   locked.session.ID,
		OrgID:       locked.session.OrgID,
		AgentID:     locked.session.AgentID,
		Status:      model.SessionReflectionStatusRunning,
		LockedUntil: ptrTime(now.Add(time.Minute)),
	}).Error; err != nil {
		t.Fatalf("create locked state: %v", err)
	}

	enq := &enqueue.MockClient{}
	handler := NewSessionReflectionScanHandler(db, enq)
	handler.now = func() time.Time { return now }
	if err := handler.Handle(context.Background(), asynq.NewTask(TypeSessionReflectionScan, nil)); err != nil {
		t.Fatalf("scan: %v", err)
	}

	ids := reflectionTaskSessionIDs(t, enq.Tasks())
	if !ids[fx.session.ID] {
		t.Fatalf("eligible session was not enqueued; ids=%v want=%s", ids, fx.session.ID)
	}
	for _, skipped := range []uuid.UUID{active.session.ID, recent.session.ID, locked.session.ID} {
		if ids[skipped] {
			t.Fatalf("ineligible session was enqueued: %s ids=%v", skipped, ids)
		}
	}
}

func reflectionTaskSessionIDs(t *testing.T, tasks []enqueue.EnqueuedTask) map[uuid.UUID]bool {
	t.Helper()
	out := make(map[uuid.UUID]bool, len(tasks))
	for _, task := range tasks {
		if task.TypeName != TypeSessionReflection {
			t.Fatalf("unexpected task type=%s tasks=%#v", task.TypeName, tasks)
		}
		var payload SessionReflectionPayload
		if err := json.Unmarshal(task.Payload, &payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		out[payload.SessionID] = true
	}
	return out
}

func TestSessionReflectionHandlerStoresMemoriesAndSuppressesDuplicates(t *testing.T) {
	db := connectTestDB(t)
	now := time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC)
	fx := seedReflectionFixture(t, db, now.Add(-10*time.Minute))
	mock := hivy.NewMockCompletionClient()
	mock.SetFallback(hivy.CompletionResponse{Message: hivy.Message{Content: `{
		"memories": [{
			"content": "Dana prefers concise implementation plans.",
			"kind": "preference",
			"tags": ["workflow"],
			"confidence": 0.92,
			"entities": ["Dana"],
			"expires_at": "",
			"source_event_ids": ["` + fx.event.ID.String() + `"]
		}]
	}`}})
	enq := &enqueue.MockClient{}
	handler := reflectionTestHandler(db, enq, mock, now)
	task, _, err := NewSessionReflectionTask(SessionReflectionPayload{SessionID: fx.session.ID})
	if err != nil {
		t.Fatalf("new task: %v", err)
	}
	if err := handler.Handle(context.Background(), task); err != nil {
		t.Fatalf("handle: %v", err)
	}
	insertReflectionEvent(t, db, fx, now.Add(-9*time.Minute), "Please keep plans short.")
	if err := handler.Handle(context.Background(), task); err != nil {
		t.Fatalf("second handle: %v", err)
	}

	var memories []model.AgentMemory
	if err := db.Where("source_session_id = ?", fx.session.ID).Find(&memories).Error; err != nil {
		t.Fatalf("load memories: %v", err)
	}
	if len(memories) != 1 {
		t.Fatalf("memories len=%d want 1: %#v", len(memories), memories)
	}
	mem := memories[0]
	if mem.ChannelID == nil || *mem.ChannelID != fx.session.ChannelID {
		t.Fatalf("memory channel=%v want %v", mem.ChannelID, fx.session.ChannelID)
	}
	if mem.MemoryFingerprint == "" || mem.EmbeddingStatus != model.AgentMemoryEmbeddingPending {
		t.Fatalf("memory fingerprint/status=%q/%q", mem.MemoryFingerprint, mem.EmbeddingStatus)
	}
	if mem.Metadata["source"] != "reflection" || mem.Metadata["actor_display_name"] != "Dana" {
		t.Fatalf("memory metadata=%#v", mem.Metadata)
	}
	entities, ok := mem.Metadata["entities"].([]any)
	if !ok || len(entities) != 1 || entities[0] != "Dana" {
		t.Fatalf("memory metadata entities=%#v", mem.Metadata["entities"])
	}
	if expiresAt, ok := mem.Metadata["expires_at"].(string); !ok || expiresAt != "" {
		t.Fatalf("memory metadata expires_at=%#v", mem.Metadata["expires_at"])
	}
	var state model.SessionReflectionState
	if err := db.First(&state, "session_id = ?", fx.session.ID).Error; err != nil {
		t.Fatalf("load state: %v", err)
	}
	if state.LastReflectedEventAt == nil || state.Status != model.SessionReflectionStatusIdle {
		t.Fatalf("state=%#v", state)
	}
}

func TestSessionReflectionHandlerRunsFinalPassOnArchivedSession(t *testing.T) {
	db := connectTestDB(t)
	now := time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC)
	// Event 30s ago: an active session would be skipped by the idle delay,
	// an archived one gets its final pass immediately.
	fx := seedReflectionFixture(t, db, now.Add(-30*time.Second))
	if err := db.Model(&model.Session{}).Where("id = ?", fx.session.ID).
		Update("status", "archived").Error; err != nil {
		t.Fatalf("archive session: %v", err)
	}
	mock := hivy.NewMockCompletionClient()
	mock.SetFallback(hivy.CompletionResponse{Message: hivy.Message{Content: `{
		"memories": [{
			"content": "Dana prefers concise implementation plans.",
			"kind": "preference",
			"tags": ["workflow"],
			"confidence": 0.92,
			"entities": ["Dana"],
			"expires_at": "",
			"source_event_ids": ["` + fx.event.ID.String() + `"]
		}]
	}`}})
	handler := reflectionTestHandler(db, &enqueue.MockClient{}, mock, now)
	task, _, err := NewSessionReflectionTask(SessionReflectionPayload{SessionID: fx.session.ID})
	if err != nil {
		t.Fatalf("new task: %v", err)
	}
	if err := handler.Handle(context.Background(), task); err != nil {
		t.Fatalf("handle: %v", err)
	}
	var memories []model.AgentMemory
	if err := db.Where("source_session_id = ?", fx.session.ID).Find(&memories).Error; err != nil {
		t.Fatalf("load memories: %v", err)
	}
	if len(memories) != 1 {
		t.Fatalf("memories len=%d want 1: %#v", len(memories), memories)
	}
	var state model.SessionReflectionState
	if err := db.First(&state, "session_id = ?", fx.session.ID).Error; err != nil {
		t.Fatalf("load state: %v", err)
	}
	if state.LastReflectedEventAt == nil || state.Status != model.SessionReflectionStatusIdle {
		t.Fatalf("state=%#v", state)
	}
}

func TestSessionReflectionHandlerDoesNotAdvanceCursorOnParseFailure(t *testing.T) {
	db := connectTestDB(t)
	now := time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC)
	fx := seedReflectionFixture(t, db, now.Add(-10*time.Minute))
	mock := hivy.NewMockCompletionClient()
	mock.SetFallback(hivy.CompletionResponse{Message: hivy.Message{Content: `not-json`}})
	handler := reflectionTestHandler(db, &enqueue.MockClient{}, mock, now)
	task, _, err := NewSessionReflectionTask(SessionReflectionPayload{SessionID: fx.session.ID})
	if err != nil {
		t.Fatalf("new task: %v", err)
	}
	if err := handler.Handle(context.Background(), task); err == nil {
		t.Fatal("expected parse failure")
	}
	var state model.SessionReflectionState
	if err := db.First(&state, "session_id = ?", fx.session.ID).Error; err != nil {
		t.Fatalf("load state: %v", err)
	}
	if state.LastReflectedEventAt != nil || state.Status != model.SessionReflectionStatusFailed {
		t.Fatalf("state advanced after failure: %#v", state)
	}
}

type reflectionFixture struct {
	user    model.User
	agent   model.Agent
	channel model.Channel
	session model.Session
	event   model.SessionEvent
}
