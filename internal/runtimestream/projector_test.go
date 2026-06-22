package runtimestream

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/runtimeevents"
)

func TestSessionEventFromRuntimeEventUsesRuntimeSequence(t *testing.T) {
	sessionID := uuid.NewString()
	orgID := uuid.NewString()
	agentID := uuid.NewString()
	sandboxID := uuid.NewString()
	occurredAt := time.Date(2026, 2, 3, 4, 5, 6, 7, time.UTC)

	row, err := sessionEventFromRuntimeEvent(Event{
		SessionID:  sessionID,
		OrgID:      orgID,
		AgentID:    agentID,
		SandboxID:  sandboxID,
		TurnID:     "turn-1",
		SpanID:     "span-1",
		RuntimeSeq: 23,
		EventID:    "evt-23",
		EventType:  "final",
		Durability: DurabilityDurable,
		Source:     "runtime",
		Payload: map[string]any{
			"text": "done",
		},
		OccurredAt: occurredAt,
	})
	if err != nil {
		t.Fatalf("sessionEventFromRuntimeEvent: %v", err)
	}
	if row.SequenceNumber != 23 {
		t.Fatalf("sequence_number = %d, want 23", row.SequenceNumber)
	}
	if row.RuntimeSeq == nil || *row.RuntimeSeq != 23 {
		t.Fatalf("runtime_seq = %#v, want 23", row.RuntimeSeq)
	}
	if row.RuntimeEventID != "evt-23" || row.EventID != "evt-23" {
		t.Fatalf("event ids = %q/%q", row.RuntimeEventID, row.EventID)
	}
	if row.TurnID != "turn-1" || row.SpanID != "span-1" || row.Durability != DurabilityDurable {
		t.Fatalf("runtime metadata = turn:%q span:%q durability:%q", row.TurnID, row.SpanID, row.Durability)
	}
	if row.EventAt != occurredAt {
		t.Fatalf("event_at = %v, want %v", row.EventAt, occurredAt)
	}
	if row.Payload["text"] != "done" {
		t.Fatalf("payload = %#v", row.Payload)
	}
}

func TestTerminalRuntimeEventsMatchQueueReleaseContract(t *testing.T) {
	releasing := []string{
		runtimeevents.EventTurnCompleted,
		runtimeevents.EventTurnFailed,
		runtimeevents.EventTurnInterrupted,
	}
	for _, eventType := range releasing {
		if !isTerminalRuntimeEvent(eventType) {
			t.Fatalf("%s should release queued session delivery", eventType)
		}
	}
	notReleasing := []string{
		runtimeevents.EventFinal,
		runtimeevents.EventDone,
		runtimeevents.EventError,
		runtimeevents.EventAgentError,
	}
	for _, eventType := range notReleasing {
		if isTerminalRuntimeEvent(eventType) {
			t.Fatalf("%s should not release queued session delivery", eventType)
		}
	}
}

func TestShouldPersistDurableRuntimeEventSkipsBackendOwnedUserMessage(t *testing.T) {
	if shouldPersistDurableRuntimeEvent(Event{EventType: runtimeevents.EventUserMessageReceived}) {
		t.Fatal("runtime user message received should be skipped because backend owns user writes")
	}
	if !shouldPersistDurableRuntimeEvent(Event{EventType: runtimeevents.EventFinal}) {
		t.Fatal("runtime final should be persisted")
	}
}
