package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/crypto"
)

// credentialFetchTimeout bounds the detached singleflight fetch so a hung
// L2/L3 lookup can't pin the flight (and its waiters) indefinitely after the
// originating request ctx is gone.
const credentialFetchTimeout = 15 * time.Second

// DecryptedCredential is the fully resolved, plaintext credential returned
// to callers of the cache manager.
type DecryptedCredential struct {
	APIKey     []byte
	BaseURL    string
	AuthScheme string
	ProviderID string
	// OrgID is the owning org of the resolved credential (uuid.Nil for
	// global credentials). Callers use it to re-assert org isolation on
	// singleflight-shared results.
	OrgID uuid.UUID
}

// Manager orchestrates the 3-tier cache: L1 (memory) → L2 (Redis) → L3 (Postgres + KMS).
type Manager struct {
	memory      *MemoryCache
	redisCache  *RedisCache
	dekCache    *DEKCache
	revokedTok  *RevokedTokenCache
	invalidator *Invalidator
	kms         *crypto.KeyWrapper
	db          *gorm.DB

	flight singleflight.Group

	// Hard expiry for L1 entries — absolute max time to serve from memory.
	hardExpiry time.Duration
}

// NewManager creates a cache manager wired to all three tiers.
func NewManager(
	memCache *MemoryCache,
	redisCache *RedisCache,
	dekCache *DEKCache,
	revokedTok *RevokedTokenCache,
	invalidator *Invalidator,
	kms *crypto.KeyWrapper,
	db *gorm.DB,
	hardExpiry time.Duration,
) *Manager {
	return &Manager{
		memory:      memCache,
		redisCache:  redisCache,
		dekCache:    dekCache,
		revokedTok:  revokedTok,
		invalidator: invalidator,
		kms:         kms,
		db:          db,
		hardExpiry:  hardExpiry,
	}
}

// GetDecryptedCredentialByID looks up a credential by id alone, reading the
// owning org from the row.
func (m *Manager) GetDecryptedCredentialByID(ctx context.Context, credentialID string) (*DecryptedCredential, error) {
	var orgIDOnly struct {
		OrgID *uuid.UUID `gorm:"column:org_id"`
	}
	if err := m.db.WithContext(ctx).
		Table("credentials").
		Select("org_id").
		Where("id = ? AND revoked_at IS NULL", credentialID).
		Take(&orgIDOnly).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("credential not found or revoked")
		}
		return nil, fmt.Errorf("db lookup: %w", err)
	}
	if orgIDOnly.OrgID == nil {
		return m.GetDecryptedCredential(ctx, credentialID, uuid.Nil)
	}
	return m.GetDecryptedCredential(ctx, credentialID, *orgIDOnly.OrgID)
}

// GetDecryptedCredential resolves a credential through the 3-tier cache.
// L1 hit → ~0.01ms, L2 hit → ~0.5ms, L3 hit → ~3-8ms.
// Uses singleflight to deduplicate concurrent requests for the same credential.
func (m *Manager) GetDecryptedCredential(ctx context.Context, credentialID string, orgID uuid.UUID) (*DecryptedCredential, error) {

	if cached, ok := m.memory.Get(credentialID); ok && cached.OrgID == orgID {
		buf, err := cached.Enclave.Open()
		if err != nil {

			m.memory.Invalidate(credentialID)
		} else {
			apiKey := make([]byte, buf.Size())
			copy(apiKey, buf.Bytes())
			buf.Destroy()
			return &DecryptedCredential{
				APIKey:     apiKey,
				BaseURL:    cached.BaseURL,
				AuthScheme: cached.AuthScheme,
				ProviderID: cached.ProviderID,
				OrgID:      cached.OrgID,
			}, nil
		}
	}

	// Key the flight by credential AND org: two orgs must never share a
	// resolution (a uuid.Nil-org credential and an org-scoped one with the
	// same ID would otherwise collide and return the wrong, unvalidated row).
	flightKey := credentialID + "|" + orgID.String()

	// Detach the shared fetch from the leader's request ctx: otherwise the
	// first caller disconnecting cancels the fetch for every waiter, 5xx-ing
	// unrelated proxy requests. Give it its own bounded deadline.
	v, err, _ := m.flight.Do(flightKey, func() (any, error) {
		fetchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), credentialFetchTimeout)
		defer cancel()
		return m.resolveFromLowerTiers(fetchCtx, credentialID, orgID)
	})
	if err != nil {
		return nil, err
	}
	cred := v.(*DecryptedCredential)
	// resolveFromDB/L2 already filter by orgID, but the result is shared
	// across all waiters on this key; re-assert the invariant defensively so
	// a future key change can never leak a credential across orgs.
	if cred.OrgID != orgID {
		return nil, fmt.Errorf("credential not found or revoked")
	}
	return cred, nil
}

// resolveFromLowerTiers checks L2 then L3, promoting results upward.
func (m *Manager) resolveFromLowerTiers(ctx context.Context, credentialID string, orgID uuid.UUID) (*DecryptedCredential, error) {

	redisCred, err := m.redisCache.Get(ctx, credentialID)
	if err != nil {

		redisCred = nil
	}

	if redisCred != nil && redisCred.OrgID == orgID.String() {

		apiKey, err := m.decryptWithDEKCache(ctx, credentialID, redisCred.EncryptedKey, redisCred.WrappedDEK)
		if err != nil {

			_ = m.redisCache.Invalidate(ctx, credentialID)
		} else {
			cred := &DecryptedCredential{
				APIKey:     apiKey,
				BaseURL:    redisCred.BaseURL,
				AuthScheme: redisCred.AuthScheme,
				ProviderID: redisCred.ProviderID,
				OrgID:      orgID,
			}

			m.promoteToL1(credentialID, orgID, apiKey, redisCred.BaseURL, redisCred.AuthScheme, redisCred.ProviderID)
			return cred, nil
		}
	}

	return m.resolveFromDB(ctx, credentialID, orgID)
}

// InvalidateCredential removes a credential from all cache tiers and
// publishes an invalidation message for other instances.
func (m *Manager) InvalidateCredential(ctx context.Context, credentialID string) error {
	m.memory.Invalidate(credentialID)
	m.dekCache.Invalidate(credentialID)

	if err := m.redisCache.Invalidate(ctx, credentialID); err != nil {
		return fmt.Errorf("redis invalidate: %w", err)
	}

	return m.invalidator.PublishCredentialInvalidation(ctx, credentialID)
}
