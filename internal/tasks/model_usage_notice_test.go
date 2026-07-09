package tasks

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

type recordingUsagePublisher struct {
	mu       sync.Mutex
	sessions map[uuid.UUID]uuid.UUID
	err      error
}

func (p *recordingUsagePublisher) PublishUsageUpdated(_ context.Context, orgID, sessionID uuid.UUID) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.sessions == nil {
		p.sessions = map[uuid.UUID]uuid.UUID{}
	}
	p.sessions[sessionID] = orgID
	return p.err
}

func (p *recordingUsagePublisher) org(sessionID uuid.UUID) (uuid.UUID, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	orgID, ok := p.sessions[sessionID]
	return orgID, ok
}

func TestPublishUsageUpdatedTargetsSessionOrg(t *testing.T) {
	orgID := uuid.New()
	sessionID := uuid.New()
	pub := &recordingUsagePublisher{}
	h := NewModelUsageHandler(nil, pub)

	h.publishUsageUpdated(context.Background(), model.Generation{OrgID: orgID, SessionID: &sessionID})

	got, ok := pub.org(sessionID)
	if !ok {
		t.Fatal("expected usage.updated notice for session-scoped generation")
	}
	if got != orgID {
		t.Fatalf("notice org = %s, want %s", got, orgID)
	}
}

func TestPublishUsageUpdatedSkipsNilSession(t *testing.T) {
	pub := &recordingUsagePublisher{}
	h := NewModelUsageHandler(nil, pub)

	h.publishUsageUpdated(context.Background(), model.Generation{OrgID: uuid.New()})

	pub.mu.Lock()
	defer pub.mu.Unlock()
	if len(pub.sessions) != 0 {
		t.Fatalf("expected no notices for nil-session generation, got %d", len(pub.sessions))
	}
}

func TestPublishUsageUpdatedSwallowsPublisherError(t *testing.T) {
	sessionID := uuid.New()
	pub := &recordingUsagePublisher{err: errors.New("redis down")}
	h := NewModelUsageHandler(nil, pub)

	h.publishUsageUpdated(context.Background(), model.Generation{OrgID: uuid.New(), SessionID: &sessionID})

	if _, ok := pub.org(sessionID); !ok {
		t.Fatal("publisher should be invoked even when it errors")
	}
}

func TestPublishUsageUpdatedNilPublisherIsNoop(t *testing.T) {
	sessionID := uuid.New()
	h := NewModelUsageHandler(nil, nil)
	h.publishUsageUpdated(context.Background(), model.Generation{OrgID: uuid.New(), SessionID: &sessionID})
}
