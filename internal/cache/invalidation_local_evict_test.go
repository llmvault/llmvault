package cache_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/cache"
)

// Revoking instance must drop the key from its own L1 cache before/while publishing, so it can
// never serve the revoked key from memory even if the cross-instance publish is delayed.
func TestInvalidateAPIKey_EvictsLocalCacheSynchronously(t *testing.T) {
	rc := connectTestRedis(t)
	apiKeyCache := cache.NewAPIKeyCache(100, 5*time.Minute)
	cfg := cache.DefaultConfig()
	mgr := cache.Build(cfg, rc, nil, nil, apiKeyCache)

	keyHash := "hash-" + uuid.NewString()
	apiKeyCache.Set(keyHash, &cache.CachedAPIKey{
		ID:     uuid.New(),
		OrgID:  uuid.New(),
		Scopes: []string{"all"},
	})
	if _, ok := apiKeyCache.Get(keyHash); !ok {
		t.Fatal("precondition: key should be cached")
	}

	if err := mgr.InvalidateAPIKey(context.Background(), keyHash); err != nil {
		t.Fatalf("InvalidateAPIKey: %v", err)
	}

	if _, ok := apiKeyCache.Get(keyHash); ok {
		t.Fatal("API key should be evicted from local cache synchronously after InvalidateAPIKey")
	}
}

// Every (re)subscribe must purge L1 so a missed-invalidation window can't keep a
// stale API key cached.
func TestSubscribe_PurgesLocalCachesOnSubscribe(t *testing.T) {
	rc := connectTestRedis(t)
	memCache := cache.NewMemoryCache(100, 5*time.Minute)
	dekCache := cache.NewDEKCache(100, 5*time.Minute)
	apiKeyCache := cache.NewAPIKeyCache(100, 5*time.Minute)
	inv := cache.NewInvalidator(rc, memCache, dekCache, apiKeyCache)

	keyHash := "hash-" + uuid.NewString()
	apiKeyCache.Set(keyHash, &cache.CachedAPIKey{ID: uuid.New(), OrgID: uuid.New(), Scopes: []string{"all"}})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = inv.Subscribe(ctx) }()

	deadline := time.After(3 * time.Second)
	for {
		if _, ok := apiKeyCache.Get(keyHash); !ok {
			return // purged on (re)subscribe — correct.
		}
		select {
		case <-deadline:
			t.Fatal("API key cache not purged after subscription became live")
		case <-time.After(20 * time.Millisecond):
		}
	}
}
