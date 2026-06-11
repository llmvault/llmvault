package mcpserver

import (
	"context"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/sync/singleflight"

	"github.com/usehivy/hivy/internal/goroutine"
)

// ServerCache caches built MCP server instances by JTI to avoid rebuilding on every request.
type ServerCache struct {
	mu      sync.RWMutex
	servers map[string]*cachedServer

	// flight deduplicates concurrent builds for the same JTI. The build runs outside c.mu so a
	// slow build for one key never blocks reads or builds for another.
	flight singleflight.Group
}

type cachedServer struct {
	server    *mcp.Server
	expiresAt time.Time
}

// NewServerCache creates a new server cache.
func NewServerCache() *ServerCache {
	return &ServerCache{
		servers: make(map[string]*cachedServer),
	}
}

// GetOrBuild returns a cached server or calls the build function to create one.
// The build function returns the server and its expiry time.
func (c *ServerCache) GetOrBuild(jti string, build func() (*mcp.Server, time.Time, error)) (*mcp.Server, error) {
	if srv, ok := c.lookup(jti); ok {
		return srv, nil
	}

	// Build (or join an in-flight build) for this JTI without holding c.mu, so
	// other keys keep being served. singleflight collapses the thundering herd
	// for the same JTI into a single build.
	v, err, _ := c.flight.Do(jti, func() (any, error) {
		// Re-check: a concurrent request may have populated the cache before this flight slot.
		if srv, ok := c.lookup(jti); ok {
			return srv, nil
		}
		srv, expiresAt, err := build()
		if err != nil {
			return nil, err
		}
		c.mu.Lock()
		c.servers[jti] = &cachedServer{server: srv, expiresAt: expiresAt}
		c.mu.Unlock()
		return srv, nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*mcp.Server), nil
}

// lookup returns a live (non-expired) cached server for jti.
func (c *ServerCache) lookup(jti string) (*mcp.Server, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if cs, ok := c.servers[jti]; ok && time.Now().Before(cs.expiresAt) {
		return cs.server, true
	}
	return nil, false
}

// Evict removes a cached server by JTI.
func (c *ServerCache) Evict(jti string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.servers, jti)
}

// StartCleanup runs a background goroutine that removes expired entries periodically.
func (c *ServerCache) StartCleanup(ctx context.Context, interval time.Duration) {
	goroutine.Go(ctx, func(ctx context.Context) {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				c.cleanup()
			}
		}
	})
}

func (c *ServerCache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for jti, cs := range c.servers {
		if now.After(cs.expiresAt) {
			delete(c.servers, jti)
		}
	}
}
