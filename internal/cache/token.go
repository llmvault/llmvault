package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/usehivy/hivy/internal/model"
)

// InvalidateToken marks a token as revoked across all tiers.
func (m *Manager) InvalidateToken(ctx context.Context, jti string, ttl time.Duration) error {

	m.invalidator.markRevoked(jti, ttl)

	if err := m.revokedTok.MarkRevoked(ctx, jti, ttl); err != nil {
		return fmt.Errorf("redis mark revoked: %w", err)
	}

	return m.invalidator.PublishTokenRevocation(ctx, jti)
}

// IsTokenRevoked checks all tiers for token revocation.
// L1 (in-memory set) -> L2 (Redis) -> L3 (Postgres).
func (m *Manager) IsTokenRevoked(ctx context.Context, jti string) (bool, error) {

	if m.invalidator.IsTokenLocallyRevoked(jti) {
		return true, nil
	}

	revoked, err := m.revokedTok.IsRevoked(ctx, jti)
	if err != nil {

		revoked = false
	}
	if revoked {

		m.invalidator.markRevoked(jti, revokedEntryMaxTTL)
		return true, nil
	}

	var count int64
	err = m.db.WithContext(ctx).
		Model(&model.Token{}).
		Where("jti = ? AND revoked_at IS NOT NULL", jti).
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("db revocation check: %w", err)
	}
	if count > 0 {

		_ = m.revokedTok.MarkRevoked(ctx, jti, 24*time.Hour)
		m.invalidator.markRevoked(jti, revokedEntryMaxTTL)
		return true, nil
	}

	return false, nil
}

// InvalidateAPIKey removes an API key from the local cache and publishes
// a cross-instance invalidation message. Local eviction is synchronous and
// happens before publishing so the revoking instance never serves the key
// from its own L1 cache, even if the Redis publish fails.
func (m *Manager) InvalidateAPIKey(ctx context.Context, keyHash string) error {
	if m.invalidator.apiKeyCache != nil {
		m.invalidator.apiKeyCache.Invalidate(keyHash)
	}
	return m.invalidator.PublishAPIKeyInvalidation(ctx, keyHash)
}
