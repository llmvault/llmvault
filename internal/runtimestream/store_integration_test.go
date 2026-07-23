package runtimestream

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/usehivy/hivy/internal/redisutil"
	"github.com/usehivy/hivy/internal/testdb"
)

func TestStoreAppendDetectsDuplicateGapAndConflict(t *testing.T) {
	ctx := context.Background()
	store, redisClient := newRedisBackedStore(t)
	sessionID := uuid.NewString()
	t.Cleanup(func() {
		_ = redisutil.Delete(ctx, redisClient, store.LastSeqKey(sessionID), store.EventIndexKey(sessionID), store.ProjectedSeqKey(sessionID))
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
		_ = redisutil.Delete(ctx, redisClient, store.LastSeqKey(sessionID), store.EventIndexKey(sessionID), store.ProjectedSeqKey(sessionID))
	})

	event := testRuntimeEvent(sessionID, 1, "evt-1", map[string]any{"text": "hello"})
	if _, err := store.Append(ctx, event); err != nil {
		t.Fatalf("append accepted: %v", err)
	}
	if err := redisClient.Del(ctx, store.EventIndexKey(sessionID)).Err(); err != nil {
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

func TestStoreAppendKeysShareRedisClusterSlot(t *testing.T) {
	ctx := context.Background()
	store, redisClient := newRedisBackedStore(t)
	if !store.IsCluster() {
		t.Skip("Redis Cluster is not configured")
	}
	sessionID := uuid.NewString()
	shard := ShardForSession(sessionID, store.ShardCount())
	keys := []string{
		store.LastSeqKey(sessionID),
		store.StreamKey(shard),
		store.EventIndexKey(sessionID),
	}
	var wantSlot int64 = -1
	for _, key := range keys {
		slot, err := redisClient.Do(ctx, "CLUSTER", "KEYSLOT", key).Int64()
		if err != nil {
			t.Fatalf("load cluster slot for %q: %v", key, err)
		}
		if wantSlot == -1 {
			wantSlot = slot
		} else if slot != wantSlot {
			t.Fatalf("append key %q slot=%d, want %d; keys=%v", key, slot, wantSlot, keys)
		}
	}
	t.Cleanup(func() {
		_ = redisutil.Delete(ctx, redisClient, keys...)
	})

	pubsub := redisClient.Subscribe(ctx, LiveChannel(sessionID))
	t.Cleanup(func() { _ = pubsub.Close() })
	if _, err := pubsub.Receive(ctx); err != nil {
		t.Fatalf("subscribe to cluster live channel: %v", err)
	}

	beforeLength, err := redisClient.XLen(ctx, store.StreamKey(shard)).Result()
	if err != nil {
		t.Fatalf("read cluster stream length before append: %v", err)
	}
	if _, err := store.Append(ctx, testRuntimeEvent(sessionID, 1, "cluster-slot-event", map[string]any{"text": "hello"})); err != nil {
		t.Fatalf("append through Redis Cluster: %v", err)
	}
	if length, err := redisClient.XLen(ctx, store.StreamKey(shard)).Result(); err != nil {
		t.Fatalf("read cluster stream length: %v", err)
	} else if length != beforeLength+1 {
		t.Fatalf("cluster stream length = %d, want %d", length, beforeLength+1)
	}
	messageCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if _, err := pubsub.ReceiveMessage(messageCtx); err != nil {
		t.Fatalf("receive live message published by append script: %v", err)
	}
}

func TestProjectorProcessesPreviewWithoutDatabaseAndCheckpoints(t *testing.T) {
	ctx := context.Background()
	store, redisClient := newRedisBackedStore(t)
	sessionID := uuid.NewString()
	stream := "runtime_events_test:" + uuid.NewString()
	t.Cleanup(func() {
		_ = redisutil.Delete(ctx, redisClient, stream, store.ProjectedSeqKey(sessionID))
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
	checkpoint, err := redisClient.Get(ctx, store.ProjectedSeqKey(sessionID)).Result()
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

func newRedisBackedStore(t *testing.T) (*Store, redis.UniversalClient) {
	t.Helper()
	client := testdb.NewRedisClient()
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
