package runtimestream

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/usehivy/hivy/internal/testdb"
)

func TestStoreAppendDetectsDuplicateGapAndConflict(t *testing.T) {
	ctx := context.Background()
	store, redisClient := newRedisBackedStore(t)
	sessionID := uuid.NewString()
	t.Cleanup(func() {
		redisClient.Del(ctx, LastSeqKey(sessionID), EventIndexKey(sessionID), ProjectedSeqKey(sessionID))
	})

	event := testRuntimeEvent(sessionID, 1, "evt-1", map[string]any{"text": "hello"})
	result, err := store.Append(ctx, event)
	if err != nil {
		t.Fatalf("append accepted: %v", err)
	}
	if result.Status != AppendAccepted || result.ExpectedSeq != 2 || result.StreamID == "" {
		t.Fatalf("accepted result = %+v", result)
	}

	result, err = store.Append(ctx, event)
	if err != nil {
		t.Fatalf("append duplicate: %v", err)
	}
	if result.Status != AppendDuplicate || !result.Duplicate || result.ExpectedSeq != 2 {
		t.Fatalf("duplicate result = %+v", result)
	}

	gap := testRuntimeEvent(sessionID, 3, "evt-3", map[string]any{"text": "later"})
	result, err = store.Append(ctx, gap)
	if err != nil {
		t.Fatalf("append gap: %v", err)
	}
	if result.Status != AppendGap || result.ExpectedSeq != 2 {
		t.Fatalf("gap result = %+v", result)
	}

	conflictID := testRuntimeEvent(sessionID, 1, "evt-other", map[string]any{"text": "hello"})
	result, err = store.Append(ctx, conflictID)
	if err != nil {
		t.Fatalf("append conflicting event_id: %v", err)
	}
	if result.Status != AppendConflict || !result.Conflict || result.Error == "" {
		t.Fatalf("event_id conflict result = %+v", result)
	}

	conflictPayload := testRuntimeEvent(sessionID, 1, "evt-1", map[string]any{"text": "changed"})
	result, err = store.Append(ctx, conflictPayload)
	if err != nil {
		t.Fatalf("append conflicting payload: %v", err)
	}
	if result.Status != AppendConflict || !result.Conflict || result.Error == "" {
		t.Fatalf("payload conflict result = %+v", result)
	}
}

func TestStoreAppendDoesNotSilentlyAcceptUnverifiableDuplicate(t *testing.T) {
	ctx := context.Background()
	store, redisClient := newRedisBackedStore(t)
	sessionID := uuid.NewString()
	t.Cleanup(func() {
		redisClient.Del(ctx, LastSeqKey(sessionID), EventIndexKey(sessionID), ProjectedSeqKey(sessionID))
	})

	event := testRuntimeEvent(sessionID, 1, "evt-1", map[string]any{"text": "hello"})
	if _, err := store.Append(ctx, event); err != nil {
		t.Fatalf("append accepted: %v", err)
	}
	if err := redisClient.Del(ctx, EventIndexKey(sessionID)).Err(); err != nil {
		t.Fatalf("drop event index: %v", err)
	}

	result, err := store.Append(ctx, event)
	if err != nil {
		t.Fatalf("append unverifiable duplicate: %v", err)
	}
	if result.Status != AppendConflict || result.Error == "" {
		t.Fatalf("unverifiable duplicate result = %+v", result)
	}
}

func TestProjectorProcessesPreviewWithoutDatabaseAndCheckpoints(t *testing.T) {
	ctx := context.Background()
	store, redisClient := newRedisBackedStore(t)
	sessionID := uuid.NewString()
	stream := "runtime_events_test:" + uuid.NewString()
	t.Cleanup(func() {
		redisClient.Del(ctx, stream, ProjectedSeqKey(sessionID))
	})
	if err := redisClient.XGroupCreateMkStream(ctx, stream, ProjectorGroup, "0").Err(); err != nil {
		t.Fatalf("create consumer group: %v", err)
	}

	event := testRuntimeEvent(sessionID, 1, "evt-preview-1", map[string]any{"token": "hello"})
	event.Durability = DurabilityPreview
	raw, err := event.Marshal()
	if err != nil {
		t.Fatalf("marshal preview event: %v", err)
	}
	if err := redisClient.XAdd(ctx, &redis.XAddArgs{
		Stream: stream,
		Values: map[string]any{"event": string(raw)},
	}).Err(); err != nil {
		t.Fatalf("xadd preview event: %v", err)
	}
	streams, err := redisClient.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    ProjectorGroup,
		Consumer: "preview-test",
		Streams:  []string{stream, ">"},
		Count:    1,
	}).Result()
	if err != nil {
		t.Fatalf("xreadgroup preview event: %v", err)
	}

	projector := NewProjector(store, nil, nil)
	if err := projector.processMessages(ctx, stream, streams[0].Messages); err != nil {
		t.Fatalf("process preview event: %v", err)
	}
	checkpoint, err := redisClient.Get(ctx, ProjectedSeqKey(sessionID)).Result()
	if err != nil {
		t.Fatalf("load projected checkpoint: %v", err)
	}
	if checkpoint != strconv.FormatInt(event.RuntimeSeq, 10) {
		t.Fatalf("projected checkpoint = %q, want %d", checkpoint, event.RuntimeSeq)
	}
	pending, err := redisClient.XPending(ctx, stream, ProjectorGroup).Result()
	if err != nil {
		t.Fatalf("xpending preview event: %v", err)
	}
	if pending.Count != 0 {
		t.Fatalf("pending count = %d, want 0", pending.Count)
	}
}

func newRedisBackedStore(t *testing.T) (*Store, *redis.Client) {
	t.Helper()
	client := redis.NewClient(&redis.Options{Addr: testdb.RedisAddr("HIVY_REDIS_ADDR", "TEST_REDIS_ADDR")})
	if err := client.Ping(context.Background()).Err(); err != nil {
		client.Close()
		t.Skipf("Redis is not available: %v", err)
	}
	t.Cleanup(func() { client.Close() })
	store := NewStore(client, 1)
	store.streamMaxLen = 1000
	store.sessionTTL = time.Hour
	return store, client
}

func testRuntimeEvent(sessionID string, seq int64, eventID string, payload map[string]any) Event {
	return Event{
		SessionID:  sessionID,
		RuntimeSeq: seq,
		EventID:    eventID,
		EventType:  "token",
		Durability: DurabilityDurable,
		Payload:    payload,
		OccurredAt: time.Date(2026, 6, 22, 12, 0, int(seq), 0, time.UTC),
	}
}
