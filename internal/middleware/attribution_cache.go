package middleware

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

type Attribution struct {
	UserID    string
	Tags      []string
	SandboxID string
	SessionID *uuid.UUID
}

type attributionEntry struct {
	attr      Attribution
	expiresAt time.Time
}

type AttributionCache struct {
	mu         sync.RWMutex
	entries    map[string]attributionEntry
	maxEntries int
	ttl        time.Duration
	now        func() time.Time
}

func NewAttributionCache(maxEntries int, ttl time.Duration) *AttributionCache {
	if maxEntries <= 0 {
		maxEntries = 1
	}
	return &AttributionCache{
		entries:    make(map[string]attributionEntry, maxEntries),
		maxEntries: maxEntries,
		ttl:        ttl,
		now:        time.Now,
	}
}

func (c *AttributionCache) Get(jti string) (Attribution, bool) {
	if c == nil || jti == "" {
		return Attribution{}, false
	}
	c.mu.RLock()
	entry, ok := c.entries[jti]
	c.mu.RUnlock()
	if !ok {
		return Attribution{}, false
	}
	if c.now().After(entry.expiresAt) {
		c.mu.Lock()
		if cur, ok := c.entries[jti]; ok && c.now().After(cur.expiresAt) {
			delete(c.entries, jti)
		}
		c.mu.Unlock()
		return Attribution{}, false
	}
	return entry.attr, true
}

func (c *AttributionCache) Set(jti string, attr Attribution) {
	if c == nil || jti == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.entries[jti]; !exists && len(c.entries) >= c.maxEntries {
		c.evictLocked()
	}
	c.entries[jti] = attributionEntry{attr: attr, expiresAt: c.now().Add(c.ttl)}
}

func (c *AttributionCache) Len() int {
	if c == nil {
		return 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

func (c *AttributionCache) evictLocked() {
	now := c.now()
	for k, e := range c.entries {
		if now.After(e.expiresAt) {
			delete(c.entries, k)
		}
	}
	for k := range c.entries {
		if len(c.entries) < c.maxEntries {
			return
		}
		delete(c.entries, k)
	}
}
