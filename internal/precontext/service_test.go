package precontext

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"

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
	lister := &fakeMemoryLister{list: func(req memory.ListRequest) []model.AgentMemory {
		return []model.AgentMemory{{
			ChannelID: &channelID,
			Content:   "Channel memory marker: CHANNEL_MEMORY_TEST. The launch codename is Helio.",
			Tags:      pq.StringArray{"launch"},
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
	if len(lister.requests) != 1 {
		t.Fatalf("memory list calls=%d, want 1", len(lister.requests))
	}
	req := lister.requests[0]
	if req.OrgID != orgID ||
		req.Scope.ChannelID == nil || *req.Scope.ChannelID != channelID ||
		!req.Scope.IncludeOrgMemories ||
		req.Scope.AllChannels ||
		req.Limit != latestOrgMemoryLimit {
		t.Fatalf("unexpected channel memory list request: %#v", req)
	}
}

func TestFormatterEnforcesTotalBudget(t *testing.T) {
	large := strings.Repeat("x", TotalBudgetBytes*2)
	out := joinSections([]string{large, large, large}, TotalBudgetBytes)
	if len(out) > TotalBudgetBytes {
		t.Fatalf("context exceeded budget: %d > %d", len(out), TotalBudgetBytes)
	}
}

type fakeMemoryLister struct {
	requests []memory.ListRequest
	list     func(memory.ListRequest) []model.AgentMemory
}

func (f *fakeMemoryLister) List(_ context.Context, req memory.ListRequest) ([]model.AgentMemory, error) {
	f.requests = append(f.requests, req)
	if f.list != nil {
		return f.list(req), nil
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
