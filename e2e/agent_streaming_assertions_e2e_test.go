package e2e

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/runtimestream"
)

func assertRuntimeRedisAndPostgresConverged(t *testing.T, ctx context.Context, db *gorm.DB, redisClient *redis.Client, sessionID string) {
	t.Helper()
	events := waitForRuntimeRedisEvents(t, ctx, redisClient, sessionID)
	var previewCount, durableCount int
	durableBySeq := map[int64]runtimestream.Event{}
	lastSeq := int64(0)
	for _, event := range events {
		if event.RuntimeSeq <= lastSeq {
			t.Fatalf("session %s redis runtime_seq regression: prev=%d next=%d", sessionID, lastSeq, event.RuntimeSeq)
		}
		if event.RuntimeSeq != lastSeq+1 {
			t.Fatalf("session %s redis runtime_seq gap: prev=%d next=%d", sessionID, lastSeq, event.RuntimeSeq)
		}
		lastSeq = event.RuntimeSeq
		switch event.Durability {
		case runtimestream.DurabilityPreview:
			previewCount++
		case runtimestream.DurabilityDurable:
			durableCount++
			durableBySeq[event.RuntimeSeq] = event
		}
	}
	if previewCount == 0 || durableCount == 0 {
		t.Fatalf("session %s redis preview=%d durable=%d events=%d", sessionID, previewCount, durableCount, len(events))
	}

	rows := waitForSessionEventRows(t, ctx, db, sessionID, len(durableBySeq))
	if len(rows) >= len(events) {
		t.Fatalf("session %s postgres rows=%d redis raw events=%d; durable coalescing did not reduce writes", sessionID, len(rows), len(events))
	}
	seenSeq := map[int64]bool{}
	for _, row := range rows {
		if row.RuntimeSeq == nil {
			t.Fatalf("session %s row %s missing runtime_seq", sessionID, row.ID)
		}
		if row.SequenceNumber != *row.RuntimeSeq {
			t.Fatalf("session %s row %s sequence_number=%d runtime_seq=%d", sessionID, row.ID, row.SequenceNumber, *row.RuntimeSeq)
		}
		if seenSeq[row.SequenceNumber] {
			t.Fatalf("session %s duplicate sequence_number=%d", sessionID, row.SequenceNumber)
		}
		seenSeq[row.SequenceNumber] = true
		event, ok := durableBySeq[*row.RuntimeSeq]
		if !ok {
			t.Fatalf("session %s postgres runtime_seq=%d not found in durable redis events", sessionID, *row.RuntimeSeq)
		}
		if row.RuntimeEventID != event.EventID || row.EventType != event.EventType {
			t.Fatalf("session %s row seq=%d got event_id/type=%s/%s want=%s/%s", sessionID, row.SequenceNumber, row.RuntimeEventID, row.EventType, event.EventID, event.EventType)
		}
	}
}

func waitForRuntimeRedisEvents(t *testing.T, ctx context.Context, redisClient *redis.Client, sessionID string) []runtimestream.Event {
	t.Helper()
	deadline := time.Now().Add(2 * time.Minute)
	var last []runtimestream.Event
	for time.Now().Before(deadline) {
		events := runtimeRedisEventsForSession(t, ctx, redisClient, sessionID)
		if hasRuntimeTerminal(events) {
			return events
		}
		last = events
		select {
		case <-ctx.Done():
			t.Fatalf("context expired waiting for redis runtime events: %v", ctx.Err())
		case <-time.After(2 * time.Second):
		}
	}
	t.Fatalf("timed out waiting for redis runtime terminal event; last=%d", len(last))
	return nil
}

func runtimeRedisEventsForSession(t *testing.T, ctx context.Context, redisClient *redis.Client, sessionID string) []runtimestream.Event {
	t.Helper()
	shard := runtimestream.ShardForSession(sessionID, runtimestream.DefaultShardCount)
	messages, err := redisClient.XRange(ctx, runtimestream.StreamKey(shard), "-", "+").Result()
	if err != nil {
		t.Fatalf("read redis stream shard=%d: %v", shard, err)
	}
	events := make([]runtimestream.Event, 0, len(messages))
	for _, message := range messages {
		event, err := runtimestream.EventFromStreamValues(message.Values)
		if err != nil {
			t.Fatalf("decode redis event %s: %v", message.ID, err)
		}
		if event.SessionID == sessionID {
			events = append(events, event)
		}
	}
	sort.Slice(events, func(i, j int) bool { return events[i].RuntimeSeq < events[j].RuntimeSeq })
	return events
}

func hasRuntimeTerminal(events []runtimestream.Event) bool {
	for _, event := range events {
		if event.Durability != runtimestream.DurabilityDurable {
			continue
		}
		if event.EventType == "turn_completed" || event.EventType == "turn_failed" || event.EventType == "done" {
			return true
		}
	}
	return false
}

func waitForSessionEventRows(t *testing.T, ctx context.Context, db *gorm.DB, sessionID string, wantDurable int) []model.SessionEvent {
	t.Helper()
	sessionUUID := uuid.MustParse(sessionID)
	deadline := time.Now().Add(2 * time.Minute)
	var rows []model.SessionEvent
	for time.Now().Before(deadline) {
		rows = rows[:0]
		if err := db.WithContext(ctx).
			Where("session_id = ? AND runtime_seq IS NOT NULL", sessionUUID).
			Order("sequence_number ASC").
			Find(&rows).Error; err != nil {
			t.Fatalf("load session_events: %v", err)
		}
		if len(rows) >= wantDurable {
			return append([]model.SessionEvent(nil), rows...)
		}
		select {
		case <-ctx.Done():
			t.Fatalf("context expired waiting for session_events: %v", ctx.Err())
		case <-time.After(2 * time.Second):
		}
	}
	t.Fatalf("timed out waiting for durable rows: got=%d want=%d", len(rows), wantDurable)
	return nil
}
