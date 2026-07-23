package runtimestream

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestEventNormalizeAndValidate(t *testing.T) {
	event := Event{
		SessionID:  uuid.NewString(),
		RuntimeSeq: 7,
		EventType:  "token",
		Payload:    nil,
		OccurredAt: time.Date(2026, 1, 2, 3, 4, 5, 6, time.FixedZone("test", 3600)),
	}
	event.Normalize()
	if event.Durability != DurabilityPreview {
		t.Fatalf("durability = %q, want preview", event.Durability)
	}
	if event.Payload == nil {
		t.Fatalf("payload was not initialized")
	}
	if event.OccurredAt.Location() != time.UTC {
		t.Fatalf("occurred_at location = %v, want UTC", event.OccurredAt.Location())
	}
	if err := event.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
}

func TestRuntimeStreamKeysAreStable(t *testing.T) {
	sessionID := "11111111-1111-1111-1111-111111111111"
	shard := ShardForSession(sessionID, DefaultShardCount)
	if shard < 0 || shard >= DefaultShardCount {
		t.Fatalf("shard = %d, want in [0,%d)", shard, DefaultShardCount)
	}
	if StreamKey(12) != "runtime_events:12" {
		t.Fatalf("stream key = %q", StreamKey(12))
	}
	if ClusterStreamKey(12) != "runtime_events:{runtime-shard-12}" {
		t.Fatalf("cluster stream key = %q", ClusterStreamKey(12))
	}
	if LiveChannel(sessionID) != "runtime_session:{11111111-1111-1111-1111-111111111111}:live" {
		t.Fatalf("live channel = %q", LiveChannel(sessionID))
	}
	if LastSeqKey(sessionID) != "runtime_session:{11111111-1111-1111-1111-111111111111}:last_seq" {
		t.Fatalf("last seq key = %q", LastSeqKey(sessionID))
	}
}

func TestParseAppendScriptResult(t *testing.T) {
	status, seq, streamID, message, err := parseAppendScriptResult([]any{"accepted", "42", "1700000000-0", ""})
	if err != nil {
		t.Fatalf("parse append result: %v", err)
	}
	if status != AppendAccepted || seq != 42 || streamID != "1700000000-0" || message != "" {
		t.Fatalf("result = %q %d %q %q", status, seq, streamID, message)
	}
}
