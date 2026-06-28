package tasks

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/cache"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/registry"
	"github.com/usehivy/hivy/internal/runtimeevents"
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
			"scope": "user",
			"visibility": "all_agents",
			"kind": "preference",
			"tags": ["workflow"],
			"confidence": 0.92,
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
	if mem.Scope != model.AgentMemoryScopeUser || mem.UserID == nil || *mem.UserID != fx.user.ID {
		t.Fatalf("memory target scope=%q user=%v", mem.Scope, mem.UserID)
	}
	if mem.MemoryFingerprint == "" || mem.EmbeddingStatus != model.AgentMemoryEmbeddingPending {
		t.Fatalf("memory fingerprint/status=%q/%q", mem.MemoryFingerprint, mem.EmbeddingStatus)
	}
	if mem.Metadata["source"] != "reflection" || mem.Metadata["actor_display_name"] != "Dana" {
		t.Fatalf("memory metadata=%#v", mem.Metadata)
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

func reflectionTestHandler(db *gorm.DB, enq *enqueue.MockClient, mock *hivy.MockCompletionClient, now time.Time) *SessionReflectionHandler {
	handler := &SessionReflectionHandler{
		db:           db,
		enqueuer:     enq,
		reg:          registry.Global(),
		cacheManager: &cache.Manager{},
		loadCredential: func(context.Context, *gorm.DB, *cache.Manager, *registry.Registry) (*sessionNameCredential, error) {
			return &sessionNameCredential{
				credential: &model.Credential{ProviderID: sessionNameProviderID},
				apiKey:     "sk-test",
				modelID:    "openai/gpt-4o-mini",
			}, nil
		},
		newCompletionClient: func(*model.Credential, string) hivy.CompletionClient { return mock },
		now:                 func() time.Time { return now },
	}
	return handler
}

func seedReflectionFixture(t *testing.T, db *gorm.DB, eventAt time.Time) reflectionFixture {
	t.Helper()
	org := model.Org{Name: "reflection-org-" + uuid.NewString()[:8], Active: true}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	user := model.User{Email: "reflection-" + uuid.NewString()[:8] + "@example.com", Name: "Dana"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	agent := model.Agent{
		OrgID:         &org.ID,
		Name:          "reflection-agent-" + uuid.NewString()[:8],
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
		Name:           "reflection-" + uuid.NewString()[:8],
		Kind:           "standard",
		Visibility:     "public",
		DefaultAgentID: agent.ID,
		Origin:         "native",
	}
	if err := db.Create(&channel).Error; err != nil {
		t.Fatalf("create channel: %v", err)
	}
	session := model.Session{
		OrgID:             org.ID,
		ChannelID:         channel.ID,
		AgentID:           agent.ID,
		CreatedBy:         &user.ID,
		Model:             agent.Model,
		ReasoningEffort:   "high",
		Source:            model.SessionSourceWeb,
		SourceResourceKey: uuid.NewString(),
		Name:              "reflection",
		Status:            "active",
		AgentTurnStatus:   model.SessionAgentTurnIdle,
		IntegrationScopes: model.JSON{},
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatalf("create session: %v", err)
	}
	fx := reflectionFixture{user: user, agent: agent, channel: channel, session: session}
	fx.event = insertReflectionEvent(t, db, fx, eventAt, "My name is Dana. Please keep implementation plans concise.")
	return fx
}

func insertReflectionEvent(t *testing.T, db *gorm.DB, fx reflectionFixture, eventAt time.Time, text string) model.SessionEvent {
	t.Helper()
	event := model.SessionEvent{
		OrgID:            fx.session.OrgID,
		SessionID:        fx.session.ID,
		AgentID:          fx.agent.ID,
		RuntimeSessionID: fx.session.ID.String(),
		EventID:          "reflection-" + uuid.NewString(),
		EventType:        runtimeevents.EventUserMessageReceived,
		ActorUserID:      &fx.user.ID,
		Source:           model.SessionSourceWeb,
		SequenceNumber:   eventAt.UnixNano(),
		Durability:       "durable",
		Payload:          model.JSON{"text": text},
		EventAt:          eventAt,
	}
	if err := db.Create(&event).Error; err != nil {
		t.Fatalf("create event: %v", err)
	}
	return event
}
