package middleware

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestAttributionCacheHitAndMiss(t *testing.T) {
	c := NewAttributionCache(10, time.Minute)

	if _, ok := c.Get("absent"); ok {
		t.Fatal("expected miss for unknown jti")
	}

	sid := uuid.New()
	c.Set("jti1", Attribution{UserID: "u1", SessionID: &sid})

	got, ok := c.Get("jti1")
	if !ok {
		t.Fatal("expected hit after set")
	}
	if got.UserID != "u1" || got.SessionID == nil || *got.SessionID != sid {
		t.Fatalf("unexpected attribution: %+v", got)
	}
}

func TestAttributionCacheTTLExpiry(t *testing.T) {
	c := NewAttributionCache(10, time.Minute)
	now := time.Unix(0, 0)
	c.now = func() time.Time { return now }

	c.Set("jti1", Attribution{UserID: "u1"})

	now = now.Add(59 * time.Second)
	if _, ok := c.Get("jti1"); !ok {
		t.Fatal("entry should still be live before TTL")
	}

	now = now.Add(2 * time.Second)
	if _, ok := c.Get("jti1"); ok {
		t.Fatal("entry should have expired past TTL")
	}
	if c.Len() != 0 {
		t.Fatalf("expired entry not purged on Get, len = %d", c.Len())
	}
}

func TestAttributionCacheBoundEviction(t *testing.T) {
	c := NewAttributionCache(3, time.Minute)

	c.Set("a", Attribution{UserID: "a"})
	c.Set("b", Attribution{UserID: "b"})
	c.Set("c", Attribution{UserID: "c"})
	if c.Len() != 3 {
		t.Fatalf("len = %d, want 3", c.Len())
	}

	c.Set("d", Attribution{UserID: "d"})
	if c.Len() > 3 {
		t.Fatalf("cache exceeded max entries: len = %d", c.Len())
	}
	if _, ok := c.Get("d"); !ok {
		t.Fatal("most recent entry should be present after eviction")
	}
}

func TestAttributionCacheEvictsExpiredBeforeArbitrary(t *testing.T) {
	c := NewAttributionCache(2, time.Minute)
	now := time.Unix(0, 0)
	c.now = func() time.Time { return now }

	c.Set("stale", Attribution{UserID: "stale"})
	now = now.Add(2 * time.Minute)
	c.Set("fresh", Attribution{UserID: "fresh"})

	c.Set("new", Attribution{UserID: "new"})

	if _, ok := c.Get("fresh"); !ok {
		t.Fatal("fresh entry should survive; expired one should be evicted first")
	}
	if _, ok := c.Get("new"); !ok {
		t.Fatal("new entry should be present")
	}
	if c.Len() > 2 {
		t.Fatalf("len = %d, want <= 2", c.Len())
	}
}

func TestAttributionCacheNegativeEntry(t *testing.T) {
	c := NewAttributionCache(10, time.Minute)

	c.Set("plain", Attribution{})

	got, ok := c.Get("plain")
	if !ok {
		t.Fatal("negative entry should be a cache hit")
	}
	if got.SessionID != nil || got.UserID != "" {
		t.Fatalf("negative entry should carry no session/user: %+v", got)
	}
}

func TestAttributionCacheNilSafe(t *testing.T) {
	var c *AttributionCache
	if _, ok := c.Get("x"); ok {
		t.Fatal("nil cache Get should miss")
	}
	c.Set("x", Attribution{})
	if c.Len() != 0 {
		t.Fatal("nil cache Len should be 0")
	}
}
