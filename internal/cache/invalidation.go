package cache

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/usehivy/hivy/internal/logging"
)

const (
	CredentialChannel = "hivy:invalidate:credential"
	TokenChannel      = "hivy:invalidate:token"
	APIKeyChannel     = "hivy:invalidate:apikey"
)

// Invalidator handles cross-instance cache invalidation via Redis pub/sub.
// When a credential or token is revoked on one instance, all other instances
// are notified to purge their L1 caches.
type Invalidator struct {
	client      *redis.Client
	memCache    *MemoryCache
	dekCache    *DEKCache
	apiKeyCache *APIKeyCache

	// Recently revoked JTIs mapped to the wall-clock time after which the entry
	// may be evicted. A token whose JWT has expired can no longer be presented, so
	// retaining its revocation past that point only leaks memory.
	revokedMu  sync.RWMutex
	revokedSet map[string]time.Time
}

// revokedEntryMaxTTL bounds how long a locally-cached revocation is retained.
// Pub/sub revocations carry only the JTI (no per-token TTL), so they default to
// this ceiling, which matches the longest token lifetime the platform issues.
const revokedEntryMaxTTL = 24 * time.Hour

func NewInvalidator(client *redis.Client, memCache *MemoryCache, dekCache *DEKCache, apiKeyCache *APIKeyCache) *Invalidator {
	return &Invalidator{
		client:      client,
		memCache:    memCache,
		dekCache:    dekCache,
		apiKeyCache: apiKeyCache,
		revokedSet:  make(map[string]time.Time),
	}
}

// markRevoked records jti as revoked for at most ttl (capped at the ceiling).
func (inv *Invalidator) markRevoked(jti string, ttl time.Duration) {
	if ttl <= 0 || ttl > revokedEntryMaxTTL {
		ttl = revokedEntryMaxTTL
	}
	expiry := time.Now().Add(ttl)
	inv.revokedMu.Lock()
	inv.revokedSet[jti] = expiry
	inv.revokedMu.Unlock()
}

// sweepRevoked drops expired entries, called from the subscribe loop so the map
// stays bounded without a dedicated goroutine.
func (inv *Invalidator) sweepRevoked() {
	now := time.Now()
	inv.revokedMu.Lock()
	for jti, expiry := range inv.revokedSet {
		if now.After(expiry) {
			delete(inv.revokedSet, jti)
		}
	}
	inv.revokedMu.Unlock()
}

// PublishCredentialInvalidation notifies all instances to evict a credential.
func (inv *Invalidator) PublishCredentialInvalidation(ctx context.Context, credentialID string) error {
	return inv.client.Publish(ctx, CredentialChannel, credentialID).Err()
}

// PublishTokenRevocation notifies all instances that a token JTI was revoked.
func (inv *Invalidator) PublishTokenRevocation(ctx context.Context, jti string) error {
	return inv.client.Publish(ctx, TokenChannel, jti).Err()
}

// PublishAPIKeyInvalidation notifies all instances to evict an API key by hash.
func (inv *Invalidator) PublishAPIKeyInvalidation(ctx context.Context, keyHash string) error {
	return inv.client.Publish(ctx, APIKeyChannel, keyHash).Err()
}

// IsTokenLocallyRevoked checks the in-memory revoked set (populated by pub/sub).
// A lapsed entry is treated as absent and lazily evicted: the underlying JWT is
// itself expired by then, so durable revocation state is no longer needed.
func (inv *Invalidator) IsTokenLocallyRevoked(jti string) bool {
	now := time.Now()
	inv.revokedMu.RLock()
	expiry, ok := inv.revokedSet[jti]
	inv.revokedMu.RUnlock()
	if !ok {
		return false
	}
	if now.After(expiry) {
		inv.revokedMu.Lock()
		// Re-check under the write lock in case it was refreshed concurrently.
		if exp, ok := inv.revokedSet[jti]; ok && now.After(exp) {
			delete(inv.revokedSet, jti)
		}
		inv.revokedMu.Unlock()
		return false
	}
	return true
}

// Subscribe listens for invalidation messages, reconnecting with backoff
// whenever the Redis pub/sub channel drops. Blocks until ctx is cancelled. On
// every (re)subscribe it purges the L1 caches: a dropped subscription may have
// missed invalidations, so purging forces re-resolution from durable L2/L3 rather
// than serving a stale revoked secret.
func (inv *Invalidator) Subscribe(ctx context.Context) error {
	const (
		minBackoff = 200 * time.Millisecond
		maxBackoff = 30 * time.Second
	)
	backoff := minBackoff
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		err := inv.subscribeOnce(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			logging.FromContext(ctx).WarnContext(ctx, "invalidation subscription dropped, reconnecting", "error", err, "backoff", backoff)
		} else {
			logging.FromContext(ctx).WarnContext(ctx, "invalidation channel closed, reconnecting", "backoff", backoff)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// subscribeOnce runs a single subscription session, returning nil on a clean
// channel close and an error on a receive/connection failure (either way the
// caller resubscribes); ctx.Err() only when ctx is cancelled.
func (inv *Invalidator) subscribeOnce(ctx context.Context) error {
	pubsub := inv.client.Subscribe(ctx, CredentialChannel, TokenChannel, APIKeyChannel)
	defer pubsub.Close()

	// Confirm the subscription is live before purging — a down connection errors
	// here and we retry without flushing the caches.
	if _, err := pubsub.Receive(ctx); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}

	inv.purgeLocal()

	ch := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-ch:
			if !ok {
				return errors.New("invalidation pubsub channel closed")
			}
			switch msg.Channel {
			case CredentialChannel:
				inv.memCache.Invalidate(msg.Payload)
				inv.dekCache.Invalidate(msg.Payload)
			case TokenChannel:
				inv.markRevoked(msg.Payload, revokedEntryMaxTTL)
				inv.sweepRevoked()
			case APIKeyChannel:
				if inv.apiKeyCache != nil {
					inv.apiKeyCache.Invalidate(msg.Payload)
				}
			default:
				logging.FromContext(ctx).WarnContext(ctx, "unknown invalidation channel", "channel", msg.Channel)
			}
		}
	}
}

// purgeLocal flushes every L1 cache on each (re)subscribe so a missed
// invalidation window can't leave a revoked credential cached.
func (inv *Invalidator) purgeLocal() {
	if inv.memCache != nil {
		inv.memCache.Purge()
	}
	if inv.dekCache != nil {
		inv.dekCache.Purge()
	}
	if inv.apiKeyCache != nil {
		inv.apiKeyCache.Purge()
	}
}
