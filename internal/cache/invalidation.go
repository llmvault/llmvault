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

	// In-memory set of recently revoked JTIs (populated by pub/sub).
	revokedMu  sync.RWMutex
	revokedSet map[string]struct{}
}

// NewInvalidator creates a new cross-instance invalidator.
func NewInvalidator(client *redis.Client, memCache *MemoryCache, dekCache *DEKCache, apiKeyCache *APIKeyCache) *Invalidator {
	return &Invalidator{
		client:      client,
		memCache:    memCache,
		dekCache:    dekCache,
		apiKeyCache: apiKeyCache,
		revokedSet:  make(map[string]struct{}),
	}
}

// PublishCredentialInvalidation notifies all instances to evict a credential.
func (inv *Invalidator) PublishCredentialInvalidation(ctx context.Context, credentialID string) error {
	return inv.client.Publish(ctx, CredentialChannel, credentialID).Err()
}

// PublishTokenRevocation notifies all instances that a token JTI was revoked.
func (inv *Invalidator) PublishTokenRevocation(ctx context.Context, jti string) error {
	return inv.client.Publish(ctx, TokenChannel, jti).Err()
}

// PublishAPIKeyInvalidation notifies all instances to evict an API key by its hash.
func (inv *Invalidator) PublishAPIKeyInvalidation(ctx context.Context, keyHash string) error {
	return inv.client.Publish(ctx, APIKeyChannel, keyHash).Err()
}

// IsTokenLocallyRevoked checks the in-memory revoked set (populated by pub/sub).
func (inv *Invalidator) IsTokenLocallyRevoked(jti string) bool {
	inv.revokedMu.RLock()
	defer inv.revokedMu.RUnlock()
	_, ok := inv.revokedSet[jti]
	return ok
}

// Subscribe listens for invalidation messages, reconnecting with backoff
// whenever the Redis pub/sub channel drops. Blocks until ctx is cancelled.
// Run this in a goroutine.
//
// On every (re)subscribe — including the first — it purges the L1 caches.
// A dropped subscription means we may have missed credential/key/token
// invalidations published while disconnected; purging forces every cached
// entry to re-resolve from L2/L3 (where the revocation is durable) rather
// than serving a stale, revoked secret until its TTL.
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
		// A clean channel close (err == nil) or a connection error both mean
		// the subscription ended without ctx cancellation: reconnect.
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

// subscribeOnce runs a single subscription session. It returns nil when the
// pub/sub channel closes cleanly and a non-nil error on a receive/connection
// failure; either way the caller resubscribes. It returns ctx.Err() only
// when ctx is cancelled.
func (inv *Invalidator) subscribeOnce(ctx context.Context) error {
	pubsub := inv.client.Subscribe(ctx, CredentialChannel, TokenChannel, APIKeyChannel)
	defer pubsub.Close()

	// Confirm the subscription is live before purging — if the connection is
	// down, Receive errors here and we retry without flushing the caches.
	if _, err := pubsub.Receive(ctx); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}

	// We may have missed messages before this (re)subscription took effect.
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
				inv.revokedMu.Lock()
				inv.revokedSet[msg.Payload] = struct{}{}
				inv.revokedMu.Unlock()
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

// purgeLocal flushes every L1 cache. Called on each (re)subscribe so a
// missed invalidation window can't leave a revoked credential/key cached.
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
