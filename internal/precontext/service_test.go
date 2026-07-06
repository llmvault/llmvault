package precontext

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/memory"
	"github.com/usehivy/hivy/internal/model"
)

func TestBuildFetchesSourcesInParallel(t *testing.T) {
	service := NewService(Config{})
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	fetch := func(text string) SourceFetcher {
		return func(context.Context, Request) (string, error) {
			started <- struct{}{}
			<-release
			return text, nil
		}
	}
	service.sessions = fetch("## Recent sessions\n- session")
	service.memories = fetch("## Memories\n- memory")

	done := make(chan []string, 1)
	go func() {
		out, _ := service.Build(context.Background(), Request{OrgID: uuid.New(), AgentID: uuid.New(), Text: "hello"})
		done <- out
	}()

	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatalf("source %d did not start", i+1)
		}
	}
	close(release)
	out := <-done
	if len(out) != 1 || !strings.Contains(out[0], "## Recent sessions") || !strings.Contains(out[0], "## Memories") {
		t.Fatalf("unexpected precontext: %#v", out)
	}
}

func TestBuildOmitsFailedSource(t *testing.T) {
	service := NewService(Config{})
	service.sessions = func(context.Context, Request) (string, error) {
		return "## Recent sessions\n- session", nil
	}
	service.memories = func(context.Context, Request) (string, error) {
		return "", errors.New("memories down")
	}

	out, err := service.Build(context.Background(), Request{OrgID: uuid.New(), AgentID: uuid.New(), Text: "hello"})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected one context string, got %#v", out)
	}
	if strings.Contains(out[0], "Memories") {
		t.Fatalf("failed source was included: %s", out[0])
	}
	if !strings.Contains(out[0], "Recent sessions") {
		t.Fatalf("successful sources missing: %s", out[0])
	}
}

func TestBuildOmitsPanickingSource(t *testing.T) {
	service := NewService(Config{})
	service.sessions = func(context.Context, Request) (string, error) {
		return "## Recent sessions\n- session", nil
	}
	service.memories = func(context.Context, Request) (string, error) {
		panic("memory dependency misconfigured")
	}

	out, err := service.Build(context.Background(), Request{OrgID: uuid.New(), AgentID: uuid.New(), Text: "hello"})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected one context string, got %#v", out)
	}
	if strings.Contains(out[0], "Memories") {
		t.Fatalf("panicking source was included: %s", out[0])
	}
	if !strings.Contains(out[0], "Recent sessions") {
		t.Fatalf("successful sources missing: %s", out[0])
	}
}

func TestBuildCacheHitAvoidsSources(t *testing.T) {
	orgID := uuid.New()
	agentID := uuid.New()
	cache := newFakeCache()
	cache.values[SessionsCacheKey(orgID, agentID)] = "## Recent sessions\n- cached"

	service := NewService(Config{Cache: cache})
	sessionCalls := 0
	service.sessions = func(context.Context, Request) (string, error) {
		sessionCalls++
		return "", nil
	}
	service.memories = func(context.Context, Request) (string, error) {
		return "", nil
	}

	out, err := service.Build(context.Background(), Request{OrgID: orgID, AgentID: agentID, Text: "hello"})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if sessionCalls != 0 {
		t.Fatalf("expected cache hit to avoid session source calls, got %d", sessionCalls)
	}
	if len(out) != 1 || strings.Count(out[0], "cached") != 1 {
		t.Fatalf("unexpected cached context: %#v", out)
	}
}

func TestBuildIncludesChannelMemories(t *testing.T) {
	orgID := uuid.New()
	channelID := uuid.New()
	lister := &fakeMemoryLister{observations: func() []model.AgentObservation {
		return []model.AgentObservation{{
			ChannelID: &channelID,
			Kind:      "org-fact",
			Content:   "Channel memory marker: CHANNEL_MEMORY_TEST. The launch codename is Helio.",
		}}
	}}
	service := NewService(Config{Memories: lister})
	service.sessions = func(context.Context, Request) (string, error) { return "", nil }

	out, err := service.Build(context.Background(), Request{
		OrgID:              orgID,
		ChannelID:          channelID,
		IncludeOrgMemories: true,
		Text:               "What is the launch codename?",
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if len(out) != 1 ||
		!strings.Contains(out[0], "## Memories") ||
		!strings.Contains(out[0], "CHANNEL_MEMORY_TEST") {
		t.Fatalf("memory context missing: %#v", out)
	}
	if len(lister.obsScopes) != 1 {
		t.Fatalf("observation calls=%d, want 1", len(lister.obsScopes))
	}
	scope := lister.obsScopes[0]
	if scope.ChannelID == nil || *scope.ChannelID != channelID ||
		!scope.IncludeOrgMemories ||
		scope.AllChannels ||
		lister.obsLimits[0] != fallbackObservationLimit {
		t.Fatalf("unexpected observation scope %#v limit %d", scope, lister.obsLimits[0])
	}
}

func TestFormatterEnforcesTotalBudget(t *testing.T) {
	large := strings.Repeat("x", TotalBudgetBytes*2)
	out := joinSections([]string{large, large, large}, TotalBudgetBytes)
	if len(out) > TotalBudgetBytes {
		t.Fatalf("context exceeded budget: %d > %d", len(out), TotalBudgetBytes)
	}
}

// fakeMemoryLister has no directives or digest, so recall falls back to the
// channel's top observations (the last non-empty recall layer).
type fakeMemoryLister struct {
	observations func() []model.AgentObservation

	obsScopes []memory.ChannelScope
	obsLimits []int
}

func (f *fakeMemoryLister) ActiveDirectives(context.Context, uuid.UUID, memory.ChannelScope) ([]model.AgentDirective, error) {
	return nil, nil
}

func (f *fakeMemoryLister) ChannelMemoryDigest(context.Context, uuid.UUID, uuid.UUID) (string, error) {
	return "", nil
}

func (f *fakeMemoryLister) TopObservations(_ context.Context, _ uuid.UUID, scope memory.ChannelScope, limit int) ([]model.AgentObservation, error) {
	f.obsScopes = append(f.obsScopes, scope)
	f.obsLimits = append(f.obsLimits, limit)
	if f.observations != nil {
		return f.observations(), nil
	}
	return nil, nil
}

type fakeCache struct {
	mu     sync.Mutex
	values map[string]string
}

func newFakeCache() *fakeCache {
	return &fakeCache{values: map[string]string{}}
}

func (c *fakeCache) Get(_ context.Context, key string) (string, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	value, ok := c.values[key]
	return value, ok, nil
}

func (c *fakeCache) Set(_ context.Context, key, value string, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.values[key] = value
	return nil
}

func (c *fakeCache) Del(_ context.Context, keys ...string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, key := range keys {
		delete(c.values, key)
	}
	return nil
}

func (c *fakeCache) DeletePrefix(_ context.Context, prefix string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for key := range c.values {
		if strings.HasPrefix(key, prefix) {
			delete(c.values, key)
		}
	}
	return nil
}
