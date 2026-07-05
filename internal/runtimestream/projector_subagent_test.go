package runtimestream

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

// TestDurableSubagentEventsPersistUnderParentSession proves that durable detail
// events emitted by the runtime with scope="subagent" are (1) not filtered out
// by the durable-persist rule, (2) converted into session_events rows that keep
// the scope + subagent payload intact, and (3) stored under the PARENT session id
// (never rerouted to the child session) in runtime-sequence order.
func TestDurableSubagentEventsPersistUnderParentSession(t *testing.T) {
	db := newSessionEventsSQLiteDB(t)

	parentSessionID := uuid.New()
	childSessionID := uuid.New()
	orgID := uuid.New()
	agentID := uuid.New()
	sandboxID := uuid.New()
	jobA := "job-" + uuid.NewString()[:8]
	jobB := "job-" + uuid.NewString()[:8]

	subagentPayload := func(jobID, text string) map[string]any {
		return map[string]any{
			"scope": "subagent",
			"text":  text,
			"subagent": map[string]any{
				"job_id":            jobID,
				"agent_name":        "researcher",
				"parent_session_id": parentSessionID.String(),
				"child_session_id":  childSessionID.String(),
			},
		}
	}

	// Two interleaved subagent jobs plus a regular parent-session token, all durable
	// and all carrying SessionID = the PARENT session.
	events := []Event{
		{
			SessionID:  parentSessionID.String(),
			OrgID:      orgID.String(),
			AgentID:    agentID.String(),
			SandboxID:  sandboxID.String(),
			RuntimeSeq: 5,
			EventID:    "evt-5",
			EventType:  "tool_call_started",
			Durability: DurabilityDurable,
			Source:     "runtime",
			Payload:    subagentPayload(jobA, "job A started"),
			OccurredAt: time.Date(2026, 7, 5, 10, 0, 0, 0, time.UTC),
		},
		{
			SessionID:  parentSessionID.String(),
			OrgID:      orgID.String(),
			AgentID:    agentID.String(),
			SandboxID:  sandboxID.String(),
			RuntimeSeq: 6,
			EventID:    "evt-6",
			EventType:  "token",
			Durability: DurabilityDurable,
			Source:     "runtime",
			Payload:    subagentPayload(jobB, "job B started"),
			OccurredAt: time.Date(2026, 7, 5, 10, 0, 1, 0, time.UTC),
		},
		{
			SessionID:  parentSessionID.String(),
			OrgID:      orgID.String(),
			AgentID:    agentID.String(),
			SandboxID:  sandboxID.String(),
			RuntimeSeq: 7,
			EventID:    "evt-7",
			EventType:  "tool_call_completed",
			Durability: DurabilityDurable,
			Source:     "runtime",
			Payload:    subagentPayload(jobA, "job A done"),
			OccurredAt: time.Date(2026, 7, 5, 10, 0, 2, 0, time.UTC),
		},
	}

	// Mirror the exact projector persistence path: filter with the durable rule,
	// convert with sessionEventFromRuntimeEvent, insert with the runtime-seq upsert.
	rows := make([]model.SessionEvent, 0, len(events))
	for _, event := range events {
		if !shouldPersistDurableRuntimeEvent(event) {
			t.Fatalf("durable subagent event %q should be persisted, but was filtered", event.EventID)
		}
		row, err := sessionEventFromRuntimeEvent(event)
		if err != nil {
			t.Fatalf("convert runtime event %q: %v", event.EventID, err)
		}
		row.ID = uuid.New()
		rows = append(rows, row)
	}
	if err := db.Clauses(runtimeSeqOnConflict()).Create(&rows).Error; err != nil {
		t.Fatalf("insert durable subagent events: %v", err)
	}

	// Reload exactly the way parent-session reload does: WHERE session_id = parent.
	var stored []model.SessionEvent
	if err := db.
		Where("session_id = ?", parentSessionID).
		Order("sequence_number ASC").
		Find(&stored).Error; err != nil {
		t.Fatalf("reload parent session events: %v", err)
	}
	if len(stored) != 3 {
		t.Fatalf("stored %d parent events, want 3", len(stored))
	}

	// None leaked to the child session id.
	var childCount int64
	if err := db.Model(&model.SessionEvent{}).
		Where("session_id = ?", childSessionID).
		Count(&childCount).Error; err != nil {
		t.Fatalf("count child session events: %v", err)
	}
	if childCount != 0 {
		t.Fatalf("child session had %d events, want 0 (events must stay parent-scoped)", childCount)
	}

	wantSeq := []int64{5, 6, 7}
	wantJob := []string{jobA, jobB, jobA}
	for i, row := range stored {
		if row.SessionID != parentSessionID {
			t.Fatalf("row %d session_id = %s, want parent %s", i, row.SessionID, parentSessionID)
		}
		if row.SequenceNumber != wantSeq[i] {
			t.Fatalf("row %d sequence_number = %d, want %d", i, row.SequenceNumber, wantSeq[i])
		}
		if row.Durability != DurabilityDurable {
			t.Fatalf("row %d durability = %q, want durable", i, row.Durability)
		}
		if scope, _ := row.Payload["scope"].(string); scope != "subagent" {
			t.Fatalf("row %d payload.scope = %v, want subagent", i, row.Payload["scope"])
		}
		sub, ok := row.Payload["subagent"].(map[string]any)
		if !ok {
			t.Fatalf("row %d payload.subagent missing or wrong type: %#v", i, row.Payload["subagent"])
		}
		if jobID, _ := sub["job_id"].(string); jobID != wantJob[i] {
			t.Fatalf("row %d payload.subagent.job_id = %v, want %s", i, sub["job_id"], wantJob[i])
		}
		if pid, _ := sub["parent_session_id"].(string); pid != parentSessionID.String() {
			t.Fatalf("row %d payload.subagent.parent_session_id = %v, want %s", i, sub["parent_session_id"], parentSessionID)
		}
	}
}

func newSessionEventsSQLiteDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := newRuntimeStreamSQLiteDB(t)
	if err := db.Exec(`
CREATE TABLE session_events (
	id text PRIMARY KEY,
	org_id text NOT NULL,
	session_id text NOT NULL,
	agent_id text NOT NULL,
	sandbox_id text,
	runtime_session_id text,
	event_id text,
	event_type text NOT NULL,
	actor_user_id text,
	source text,
	sequence_number integer,
	runtime_seq bigint,
	runtime_event_id text,
	turn_id text,
	span_id text,
	durability text,
	payload jsonb,
	event_at datetime,
	retained_at datetime,
	created_at datetime
)`).Error; err != nil {
		t.Fatalf("create session_events table: %v", err)
	}
	if err := db.Exec(`
CREATE UNIQUE INDEX idx_session_events_runtime_seq
	ON session_events (session_id, runtime_seq)
	WHERE runtime_seq IS NOT NULL`).Error; err != nil {
		t.Fatalf("create runtime_seq index: %v", err)
	}
	return db
}
