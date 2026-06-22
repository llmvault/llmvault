package handler

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/runtimestream"
	"github.com/usehivy/hivy/internal/testdb"
)

func TestRuntimeIngressRawFrameAllowsRouteSessionEnrichment(t *testing.T) {
	raw, err := json.Marshal(runtimestream.Event{
		RuntimeSeq: 1,
		EventID:    "evt-1",
		EventType:  "token",
		Durability: runtimestream.DurabilityDurable,
		Payload:    map[string]any{"text": "hello"},
		OccurredAt: time.Date(2026, 6, 22, 14, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("marshal raw frame: %v", err)
	}

	events, err := decodeRuntimeIngressEvents(raw)
	if err != nil {
		t.Fatalf("decode raw frame: %v", err)
	}
	if len(events) != 1 || events[0].SessionID != "" || events[0].RuntimeSeq != 1 {
		t.Fatalf("decoded events = %+v", events)
	}
}

func TestRuntimeIngressAckIdentifiesSessionAndConflict(t *testing.T) {
	ctx := context.Background()
	redisClient := redis.NewClient(&redis.Options{Addr: testdb.RedisAddr("HIVY_REDIS_ADDR", "TEST_REDIS_ADDR")})
	if err := redisClient.Ping(ctx).Err(); err != nil {
		redisClient.Close()
		t.Skipf("Redis is not available: %v", err)
	}
	t.Cleanup(func() { redisClient.Close() })

	store := runtimestream.NewStore(redisClient, 1)
	h := &RuntimeStreamIngressHandler{store: store}
	orgID := uuid.New()
	agentID := uuid.New()
	sandboxID := uuid.New()
	sessionID := uuid.New()
	t.Cleanup(func() {
		redisClient.Del(ctx,
			runtimestream.LastSeqKey(sessionID.String()),
			runtimestream.EventIndexKey(sessionID.String()),
			runtimestream.ProjectedSeqKey(sessionID.String()),
		)
	})

	sb := &model.Sandbox{ID: sandboxID, OrgID: &orgID, AgentID: &agentID}
	session := &model.Session{ID: sessionID, OrgID: orgID, AgentID: agentID, SandboxID: &sandboxID}

	accepted, closeAfterWrite := h.acceptRuntimeEvents(ctx, sb, session, []runtimestream.Event{
		{
			RuntimeSeq: 1,
			EventID:    "evt-1",
			EventType:  "token",
			Durability: runtimestream.DurabilityDurable,
			Payload:    map[string]any{"text": "hello"},
			OccurredAt: time.Date(2026, 6, 22, 14, 0, 0, 0, time.UTC),
		},
	})
	if closeAfterWrite || accepted.Type != "ack" || len(accepted.Results) != 1 {
		t.Fatalf("accepted ack = %+v close=%t", accepted, closeAfterWrite)
	}
	if got := accepted.Results[0]; got.SessionID != sessionID.String() || got.RuntimeSeq != 1 || got.Status != runtimestream.AppendAccepted {
		t.Fatalf("accepted result = %+v", got)
	}

	conflict, closeAfterWrite := h.acceptRuntimeEvents(ctx, sb, session, []runtimestream.Event{
		{
			RuntimeSeq: 1,
			EventID:    "evt-1",
			EventType:  "token",
			Durability: runtimestream.DurabilityDurable,
			Payload:    map[string]any{"text": "changed"},
			OccurredAt: time.Date(2026, 6, 22, 14, 0, 1, 0, time.UTC),
		},
	})
	if !closeAfterWrite || conflict.Type != "nack" || len(conflict.Results) != 1 {
		t.Fatalf("conflict ack = %+v close=%t", conflict, closeAfterWrite)
	}
	if got := conflict.Results[0]; got.SessionID != sessionID.String() || got.RuntimeSeq != 1 || got.Status != runtimestream.AppendConflict || got.Error == "" {
		t.Fatalf("conflict result = %+v", got)
	}
}
