package precontext

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestBuildFetchesSourcesInParallel(t *testing.T) {
	service := NewService(Config{})
	started := make(chan struct{}, 3)
	release := make(chan struct{})
	fetch := func(text string) SourceFetcher {
		return func(context.Context, Request) (string, error) {
			started <- struct{}{}
			<-release
			return text, nil
		}
	}
	service.sessions = fetch("## Recent sessions\n- session")
	service.memories = fetch("## Recent memories\n- memory")
	service.knowledge = fetch("## Relevant knowledge\n- knowledge")

	done := make(chan []string, 1)
	go func() {
		out, _ := service.Build(context.Background(), Request{OrgID: uuid.New(), AgentID: uuid.New(), Text: "hello"})
		done <- out
	}()

	for i := 0; i < 3; i++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatalf("source %d did not start", i+1)
		}
	}
	close(release)
	out := <-done
	if len(out) != 1 || !strings.Contains(out[0], "## Recent sessions") || !strings.Contains(out[0], "## Recent memories") || !strings.Contains(out[0], "## Relevant knowledge") {
		t.Fatalf("unexpected precontext: %#v", out)
	}
}

func TestBuildOmitsFailedSource(t *testing.T) {
	service := NewService(Config{})
	service.sessions = func(context.Context, Request) (string, error) {
		return "## Recent sessions\n- session", nil
	}
	service.memories = func(context.Context, Request) (string, error) {
		return "", errors.New("hindsight down")
	}
	service.knowledge = func(context.Context, Request) (string, error) {
		return "## Relevant knowledge\n- knowledge", nil
	}

	out, err := service.Build(context.Background(), Request{OrgID: uuid.New(), AgentID: uuid.New(), Text: "hello"})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected one context string, got %#v", out)
	}
	if strings.Contains(out[0], "Recent memories") {
		t.Fatalf("failed source was included: %s", out[0])
	}
	if !strings.Contains(out[0], "Recent sessions") || !strings.Contains(out[0], "Relevant knowledge") {
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
	service.knowledge = func(context.Context, Request) (string, error) {
		return "## Relevant knowledge\n- knowledge", nil
	}

	out, err := service.Build(context.Background(), Request{OrgID: uuid.New(), AgentID: uuid.New(), Text: "hello"})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected one context string, got %#v", out)
	}
	if strings.Contains(out[0], "Recent memories") {
		t.Fatalf("panicking source was included: %s", out[0])
	}
	if !strings.Contains(out[0], "Recent sessions") || !strings.Contains(out[0], "Relevant knowledge") {
		t.Fatalf("successful sources missing: %s", out[0])
	}
}

func TestBuildCacheHitAvoidsSources(t *testing.T) {
	orgID := uuid.New()
	agentID := uuid.New()
	cache := newFakeCache()
	cache.values[SessionsCacheKey(orgID, agentID)] = "## Recent sessions\n- cached"
	cache.values[MemoriesCacheKey(orgID, agentID)] = "## Recent memories\n- cached"
	cache.values[KnowledgeCacheKey(orgID, agentID, "hello")] = "## Relevant knowledge\n- cached"

	service := NewService(Config{Cache: cache})
	calls := 0
	source := func(context.Context, Request) (string, error) {
		calls++
		return "", nil
	}
	service.sessions = source
	service.memories = source
	service.knowledge = source

	out, err := service.Build(context.Background(), Request{OrgID: orgID, AgentID: agentID, Text: "hello"})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if calls != 0 {
		t.Fatalf("expected cache hit to avoid source calls, got %d", calls)
	}
	if len(out) != 1 || strings.Count(out[0], "cached") != 3 {
		t.Fatalf("unexpected cached context: %#v", out)
	}
}

func TestFormatterEnforcesTotalBudget(t *testing.T) {
	large := strings.Repeat("x", TotalBudgetBytes*2)
	out := joinSections([]string{large, large, large}, TotalBudgetBytes)
	if len(out) > TotalBudgetBytes {
		t.Fatalf("context exceeded budget: %d > %d", len(out), TotalBudgetBytes)
	}
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
