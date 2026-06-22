package runtimestream

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/runtimeevents"
	"github.com/usehivy/hivy/internal/tasks"
)

func TestProjectSessionStateReleasesQueueOnlyForTurnTerminalEvents(t *testing.T) {
	ctx := context.Background()
	db := newRuntimeStreamSQLiteDB(t)

	tests := []struct {
		name       string
		eventType  string
		wantStatus string
		wantResult string
		wantTask   bool
	}{
		{
			name:       "final does not release",
			eventType:  runtimeevents.EventFinal,
			wantStatus: model.SessionAgentTurnActive,
		},
		{
			name:       "done does not release",
			eventType:  runtimeevents.EventDone,
			wantStatus: model.SessionAgentTurnActive,
		},
		{
			name:       "generic error does not release",
			eventType:  runtimeevents.EventError,
			wantStatus: model.SessionAgentTurnActive,
		},
		{
			name:       "turn completed releases",
			eventType:  runtimeevents.EventTurnCompleted,
			wantStatus: model.SessionAgentTurnIdle,
			wantResult: model.SessionAgentTurnOutcomeDone,
			wantTask:   true,
		},
		{
			name:       "turn failed releases",
			eventType:  runtimeevents.EventTurnFailed,
			wantStatus: model.SessionAgentTurnIdle,
			wantResult: model.SessionAgentTurnOutcomeFailed,
			wantTask:   true,
		},
		{
			name:       "turn interrupted releases",
			eventType:  runtimeevents.EventTurnInterrupted,
			wantStatus: model.SessionAgentTurnIdle,
			wantResult: model.SessionAgentTurnOutcomeStopped,
			wantTask:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			enq := &enqueue.MockClient{}
			projector := NewProjector(nil, db, nil).WithEnqueuer(enq)
			session := seedRuntimeStreamSession(t, db)
			if err := projector.projectSessionState(ctx, Event{
				SessionID:  session.ID.String(),
				RuntimeSeq: 2,
				EventType:  tc.eventType,
				Durability: DurabilityDurable,
				OccurredAt: time.Date(2026, 6, 22, 13, 0, 0, 0, time.UTC),
			}); err != nil {
				t.Fatalf("project session state: %v", err)
			}

			var stored model.Session
			if err := db.First(&stored, "id = ?", session.ID).Error; err != nil {
				t.Fatalf("load session: %v", err)
			}
			if stored.AgentTurnStatus != tc.wantStatus {
				t.Fatalf("agent_turn_status = %q, want %q", stored.AgentTurnStatus, tc.wantStatus)
			}
			if stored.AgentTurnLastOutcome != tc.wantResult {
				t.Fatalf("agent_turn_last_outcome = %q, want %q", stored.AgentTurnLastOutcome, tc.wantResult)
			}
			gotTask := false
			for _, task := range enq.Tasks() {
				gotTask = gotTask || task.TypeName == tasks.TypeSessionMessageDeliver
			}
			if gotTask != tc.wantTask {
				t.Fatalf("session delivery task enqueued = %t, want %t", gotTask, tc.wantTask)
			}
		})
	}
}

func newRuntimeStreamSQLiteDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec(`
CREATE TABLE sessions (
	id text PRIMARY KEY,
	org_id text NOT NULL,
	channel_id text NOT NULL,
	agent_id text NOT NULL,
	model text,
	access_mode text,
	reasoning_effort text,
	source text,
	name text,
	status text,
	agent_turn_status text,
	agent_turn_id text,
	agent_stream_id text,
	agent_turn_started_at datetime,
	agent_turn_last_outcome text,
	updated_at datetime
)`).Error; err != nil {
		t.Fatalf("create sessions table: %v", err)
	}
	if err := db.Exec(`
CREATE TABLE session_message_queue (
	id text PRIMARY KEY,
	org_id text NOT NULL,
	session_id text NOT NULL,
	message_text text,
	message_payload json,
	sequence_number integer,
	status text
)`).Error; err != nil {
		t.Fatalf("create session_message_queue table: %v", err)
	}
	return db
}

func seedRuntimeStreamSession(t *testing.T, db *gorm.DB) model.Session {
	t.Helper()
	session := model.Session{
		ID:                   uuid.New(),
		OrgID:                uuid.New(),
		ChannelID:            uuid.New(),
		AgentID:              uuid.New(),
		Model:                "test-model",
		AccessMode:           "full",
		ReasoningEffort:      "high",
		Source:               "web",
		Name:                 "runtime stream test",
		Status:               "active",
		AgentTurnStatus:      model.SessionAgentTurnActive,
		AgentTurnID:          "turn-1",
		AgentStreamID:        "stream-1",
		AgentTurnLastOutcome: "",
		IntegrationScopes:    model.JSON{},
	}
	if err := db.Exec(`
INSERT INTO sessions (
	id, org_id, channel_id, agent_id, model, access_mode, reasoning_effort, source,
	name, status, agent_turn_status, agent_turn_id, agent_stream_id, agent_turn_last_outcome
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		session.ID, session.OrgID, session.ChannelID, session.AgentID, session.Model, session.AccessMode,
		session.ReasoningEffort, session.Source, session.Name, session.Status, session.AgentTurnStatus,
		session.AgentTurnID, session.AgentStreamID, session.AgentTurnLastOutcome,
	).Error; err != nil {
		t.Fatalf("create session: %v", err)
	}
	queue := model.SessionMessageQueue{
		ID:             uuid.New(),
		OrgID:          session.OrgID,
		SessionID:      session.ID,
		MessageText:    "queued",
		MessagePayload: model.JSON{"text": "queued"},
		SequenceNumber: 1,
		Status:         "pending",
	}
	if err := db.Exec(`
INSERT INTO session_message_queue (
	id, org_id, session_id, message_text, message_payload, sequence_number, status
) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		queue.ID, queue.OrgID, queue.SessionID, queue.MessageText, `{"text":"queued"}`, queue.SequenceNumber, queue.Status,
	).Error; err != nil {
		t.Fatalf("create queue row: %v", err)
	}
	return session
}
