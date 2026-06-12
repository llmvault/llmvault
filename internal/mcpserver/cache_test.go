package mcpserver

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func newTestServer() *mcp.Server {
	return mcp.NewServer(&mcp.Implementation{Name: "test", Version: "v0"}, nil)
}

// A slow build for one JTI does not block a build/serve for a different JTI (build no longer runs
// under the global write lock).
func TestServerCache_PerKeySingleflight(t *testing.T) {
	c := NewServerCache()

	slowStarted := make(chan struct{})
	releaseSlow := make(chan struct{})

	slowDone := make(chan struct{})
	go func() {
		_, _ = c.GetOrBuild("slow", func() (*mcp.Server, time.Time, error) {
			close(slowStarted)
			<-releaseSlow
			return newTestServer(), time.Now().Add(time.Hour), nil
		})
		close(slowDone)
	}()

	<-slowStarted // ensure the slow build holds its key's flight slot

	fastDone := make(chan struct{})
	go func() {
		_, err := c.GetOrBuild("fast", func() (*mcp.Server, time.Time, error) {
			return newTestServer(), time.Now().Add(time.Hour), nil
		})
		if err != nil {
			t.Errorf("fast build: %v", err)
		}
		close(fastDone)
	}()

	select {
	case <-fastDone:
	case <-time.After(2 * time.Second):
		t.Fatal("fast build was blocked by the in-flight slow build (head-of-line stall)")
	}

	close(releaseSlow)
	<-slowDone
}

// Concurrent requests for the same JTI must trigger exactly one build.
func TestServerCache_SingleflightDedupesSameKey(t *testing.T) {
	c := NewServerCache()

	var builds atomic.Int32
	release := make(chan struct{})

	const n = 8
	var wg sync.WaitGroup
	results := make([]*mcp.Server, n)
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			srv, err := c.GetOrBuild("same", func() (*mcp.Server, time.Time, error) {
				builds.Add(1)
				<-release
				return newTestServer(), time.Now().Add(time.Hour), nil
			})
			if err != nil {
				t.Errorf("build %d: %v", i, err)
			}
			results[i] = srv
		}(i)
	}

	time.Sleep(100 * time.Millisecond)
	close(release)
	wg.Wait()

	if got := builds.Load(); got != 1 {
		t.Fatalf("build ran %d times, want 1 (singleflight not deduping)", got)
	}
	for i := 1; i < n; i++ {
		if results[i] != results[0] {
			t.Fatalf("callers got different server instances")
		}
	}
}

// Cache hits skip the build; expired entries rebuild.
func TestServerCache_ServesCachedAndExpires(t *testing.T) {
	c := NewServerCache()

	var builds atomic.Int32
	build := func(exp time.Time) func() (*mcp.Server, time.Time, error) {
		return func() (*mcp.Server, time.Time, error) {
			builds.Add(1)
			return newTestServer(), exp, nil
		}
	}

	if _, err := c.GetOrBuild("k", build(time.Now().Add(time.Hour))); err != nil {
		t.Fatalf("first build: %v", err)
	}
	if _, err := c.GetOrBuild("k", build(time.Now().Add(time.Hour))); err != nil {
		t.Fatalf("cached get: %v", err)
	}
	if got := builds.Load(); got != 1 {
		t.Fatalf("cache hit still rebuilt: builds=%d", got)
	}

	c.Evict("k")
	if _, err := c.GetOrBuild("k", build(time.Now().Add(-time.Minute))); err != nil {
		t.Fatalf("rebuild after evict: %v", err)
	}
	if _, err := c.GetOrBuild("k", build(time.Now().Add(time.Hour))); err != nil {
		t.Fatalf("rebuild after expiry: %v", err)
	}
	if got := builds.Load(); got != 3 {
		t.Fatalf("expected 3 builds (initial + post-evict + post-expiry), got %d", got)
	}
}
