package tasks

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

type sessionEventsNoticeCall struct {
	orgID     uuid.UUID
	sessionID uuid.UUID
	eventID   string
	eventAt   time.Time
}

type fakeSessionEventsNoticePublisher struct {
	mu    sync.Mutex
	calls []sessionEventsNoticeCall
}

func (f *fakeSessionEventsNoticePublisher) PublishSessionEventsAppended(_ context.Context, orgID, sessionID uuid.UUID, eventID string, eventAt time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, sessionEventsNoticeCall{orgID: orgID, sessionID: sessionID, eventID: eventID, eventAt: eventAt})
	return nil
}

func (f *fakeSessionEventsNoticePublisher) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

// A newly created automated user-message row publishes exactly one
// session.events.appended notice; a deduped redelivery of the same event
// reuses the existing row and publishes none.
func TestEnqueueTriggerSessionMessagePublishesNoticeOnlyOnCreate(t *testing.T) {
	db := connectTestDB(t)
	org, agent, _ := seedTriggerSessionFixture(t, db)
	channel := seedTriggerChannel(t, db, org.ID, agent.ID, "queue")
	trigger := seedTriggerForSession(t, db, org.ID, agent.ID, &channel.ID)
	notices := &fakeSessionEventsNoticePublisher{}
	handler := &AgentTriggerDispatchHandler{db: db, enqueuer: &fakeTaskEnqueuer{}, sessionEventNotices: notices}
	ctx := context.Background()

	issueKey := "github/acme/repo/issue/99"
	session, err := handler.findOrCreateTriggerSession(ctx, &agent, trigger, issueKey)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	created, err := handler.enqueueTriggerSessionMessage(ctx, session, compiledMessage(issueKey, "first"), "evt-1", triggerConversationSource)
	if err != nil {
		t.Fatalf("first enqueue: %v", err)
	}
	if !created {
		t.Fatalf("first enqueue created = false, want true")
	}
	if got := notices.count(); got != 1 {
		t.Fatalf("notices after create = %d, want 1", got)
	}
	call := notices.calls[0]
	if call.orgID != org.ID {
		t.Fatalf("notice org = %s, want %s", call.orgID, org.ID)
	}
	if call.sessionID != session.ID {
		t.Fatalf("notice session = %s, want %s", call.sessionID, session.ID)
	}
	if call.eventID == "" {
		t.Fatalf("notice event id is empty")
	}
	if call.eventAt.IsZero() {
		t.Fatalf("notice event_at is zero")
	}

	createdAgain, err := handler.enqueueTriggerSessionMessage(ctx, session, compiledMessage(issueKey, "first"), "evt-1", triggerConversationSource)
	if err != nil {
		t.Fatalf("redeliver enqueue: %v", err)
	}
	if createdAgain {
		t.Fatalf("redeliver created = true, want false")
	}
	if got := notices.count(); got != 1 {
		t.Fatalf("notices after redelivery = %d, want 1", got)
	}
}
