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
	service.knowledge = fetch("## Relevant knowledge\n- knowledge")

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
	if len(out) != 1 || !strings.Contains(out[0], "## Recent sessions") || !strings.Contains(out[0], "## Relevant knowledge") {
		t.Fatalf("unexpected precontext: %#v", out)
	}
}

func TestBuildOmitsFailedSource(t *testing.T) {
	service := NewService(Config{})
	service.sessions = func(context.Context, Request) (string, error) {
		return "## Recent sessions\n- session", nil
	}
	service.knowledge = func(context.Context, Request) (string, error) {
		return "", errors.New("knowledge down")
	}

	out, err := service.Build(context.Background(), Request{OrgID: uuid.New(), AgentID: uuid.New(), Text: "hello"})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected one context string, got %#v", out)
	}
	if strings.Contains(out[0], "Relevant knowledge") {
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
	service.knowledge = func(context.Context, Request) (string, error) {
		panic("knowledge dependency misconfigured")
	}

	out, err := service.Build(context.Background(), Request{OrgID: uuid.New(), AgentID: uuid.New(), Text: "hello"})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected one context string, got %#v", out)
	}
	if strings.Contains(out[0], "Relevant knowledge") {
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
	cache.values[KnowledgeCacheKey(orgID, agentID, "hello")] = "## Relevant knowledge\n- cached"

	service := NewService(Config{Cache: cache})
	calls := 0
	source := func(context.Context, Request) (string, error) {
		calls++
		return "", nil
	}
	service.sessions = source
	service.knowledge = source

	out, err := service.Build(context.Background(), Request{OrgID: orgID, AgentID: agentID, Text: "hello"})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if calls != 0 {
		t.Fatalf("expected cache hit to avoid source calls, got %d", calls)
	}
	if len(out) != 1 || strings.Count(out[0], "cached") != 2 {
		t.Fatalf("unexpected cached context: %#v", out)
	}
}

func TestBuildIncludesMemorySource(t *testing.T) {
	orgID := uuid.New()
	agentID := uuid.New()
	userID := uuid.New()
	service := NewService(Config{
		Memories: fakeMemorySearcher{turn: memory.TurnMemories{
			Org: []memory.SearchHit{{
				Memory: model.AgentMemory{
					Scope:   model.AgentMemoryScopeOrg,
					Content: "Organization memory marker: ORG_MEMORY_TEST. The launch codename is Helio.",
					Tags:    pq.StringArray{"launch"},
				},
			}},
			User: []memory.SearchHit{{
				Memory: model.AgentMemory{
					Scope:   model.AgentMemoryScopeUser,
					Content: "User memory marker: USER_MEMORY_TEST. The preferred escalation word is Prism.",
					Tags:    pq.StringArray{"escalation"},
				},
			}},
		}},
	})
	service.sessions = func(context.Context, Request) (string, error) { return "", nil }
	service.knowledge = func(context.Context, Request) (string, error) { return "", nil }

	out, err := service.Build(context.Background(), Request{
		OrgID:   orgID,
		AgentID: agentID,
		UserID:  userID.String(),
		Text:    "What are the launch codename and escalation word?",
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if len(out) != 1 || !strings.Contains(out[0], "## Relevant memories") ||
		!strings.Contains(out[0], "ORG_MEMORY_TEST") ||
		!strings.Contains(out[0], "USER_MEMORY_TEST") {
		t.Fatalf("memory context missing: %#v", out)
	}
}

func TestFormatterEnforcesTotalBudget(t *testing.T) {
	large := strings.Repeat("x", TotalBudgetBytes*2)
	out := joinSections([]string{large, large, large}, TotalBudgetBytes)
	if len(out) > TotalBudgetBytes {
		t.Fatalf("context exceeded budget: %d > %d", len(out), TotalBudgetBytes)
	}
}

type fakeMemorySearcher struct {
	turn memory.TurnMemories
}

func (f fakeMemorySearcher) SearchForTurn(context.Context, memory.SearchRequest) (memory.TurnMemories, error) {
	return f.turn, nil
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
