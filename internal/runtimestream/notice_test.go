package runtimestream

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

func TestPublishNoticeRoundTrip(t *testing.T) {
	ctx := context.Background()
	store, redisClient := newRedisBackedStore(t)
	sessionID := uuid.New()

	sub := redisClient.Subscribe(ctx, LiveChannel(sessionID.String()))
	defer sub.Close()
	if _, err := sub.Receive(ctx); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	ch := sub.Channel(redis.WithChannelSize(8))

	notice := Notice{
		Type:  NoticeTypeArtifactSynced,
		OrgID: uuid.New(),
		Data:  json.RawMessage(`{"artifact_id":"abc"}`),
	}
	if err := store.PublishNotice(ctx, sessionID, notice); err != nil {
		t.Fatalf("publish notice: %v", err)
	}

	msg := readNoticeMessage(t, ch)
	if msg.Kind != LiveKindNotice {
		t.Fatalf("kind = %q, want notice", msg.Kind)
	}
	if msg.SessionID != sessionID.String() {
		t.Fatalf("session id = %q, want %q", msg.SessionID, sessionID.String())
	}
	if msg.Notice == nil || msg.Notice.Type != NoticeTypeArtifactSynced {
		t.Fatalf("notice = %+v", msg.Notice)
	}
	if msg.Notice.PublishedAt.IsZero() {
		t.Fatalf("published_at was not stamped")
	}
	if string(msg.Notice.Data) != `{"artifact_id":"abc"}` {
		t.Fatalf("notice data = %s", msg.Notice.Data)
	}
}

func TestPublishNoticeStampsProvidedPublishedAt(t *testing.T) {
	ctx := context.Background()
	store, redisClient := newRedisBackedStore(t)
	sessionID := uuid.New()

	sub := redisClient.Subscribe(ctx, LiveChannel(sessionID.String()))
	defer sub.Close()
	if _, err := sub.Receive(ctx); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	ch := sub.Channel(redis.WithChannelSize(8))

	stamped := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	notice := Notice{
		Type:        NoticeTypeUsageUpdated,
		OrgID:       uuid.New(),
		SessionID:   &sessionID,
		Data:        json.RawMessage(`{"session_id":"` + sessionID.String() + `"}`),
		PublishedAt: stamped,
	}
	if err := store.PublishNotice(ctx, sessionID, notice); err != nil {
		t.Fatalf("publish notice: %v", err)
	}

	msg := readNoticeMessage(t, ch)
	if msg.Kind != LiveKindNotice {
		t.Fatalf("kind = %q, want notice", msg.Kind)
	}
	if msg.SessionID != sessionID.String() {
		t.Fatalf("session id = %q, want %q", msg.SessionID, sessionID.String())
	}
	if msg.Notice == nil || msg.Notice.Type != NoticeTypeUsageUpdated {
		t.Fatalf("notice = %+v", msg.Notice)
	}
	if !msg.Notice.PublishedAt.Equal(stamped) {
		t.Fatalf("published_at = %v, want %v", msg.Notice.PublishedAt, stamped)
	}
}

func readNoticeMessage(t *testing.T, ch <-chan *redis.Message) LiveMessage {
	t.Helper()
	select {
	case raw := <-ch:
		var msg LiveMessage
		if err := json.Unmarshal([]byte(raw.Payload), &msg); err != nil {
			t.Fatalf("decode live message: %v", err)
		}
		return msg
	case <-time.After(5 * time.Second):
		t.Fatalf("notice never arrived")
		return LiveMessage{}
	}
}
