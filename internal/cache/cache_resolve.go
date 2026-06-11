package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/awnumar/memguard"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/model"
)

// resolveFromDB fetches from Postgres, decrypts via KMS, and promotes to L2 + L1.
func (m *Manager) resolveFromDB(ctx context.Context, credentialID string, orgID uuid.UUID) (*DecryptedCredential, error) {
	var dbCred model.Credential
	query := m.db.WithContext(ctx).Where("id = ? AND revoked_at IS NULL", credentialID)
	if orgID == uuid.Nil {
		query = query.Where("org_id IS NULL")
	} else {
		query = query.Where("org_id = ?", orgID)
	}
	err := query.First(&dbCred).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("credential not found or revoked")
		}
		return nil, fmt.Errorf("db lookup: %w", err)
	}

	dek, err := m.kms.Unwrap(ctx, dbCred.WrappedDEK)
	if err != nil {
		return nil, fmt.Errorf("kms unwrap: %w", err)
	}

	apiKey, err := crypto.DecryptCredential(dbCred.EncryptedKey, dek)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}

	dekEnclave := memguard.NewEnclave(dek)
	m.dekCache.Set(credentialID, dekEnclave)

	for i := range dek {
		dek[i] = 0
	}

	_ = m.redisCache.Set(ctx, credentialID, &RedisCredential{
		EncryptedKey: dbCred.EncryptedKey,
		WrappedDEK:   dbCred.WrappedDEK,
		BaseURL:      dbCred.BaseURL,
		AuthScheme:   dbCred.AuthScheme,
		ProviderID:   dbCred.ProviderID,
		OrgID:        orgID.String(),
	})

	m.promoteToL1(credentialID, orgID, apiKey, dbCred.BaseURL, dbCred.AuthScheme, dbCred.ProviderID)

	return &DecryptedCredential{
		APIKey:     apiKey,
		BaseURL:    dbCred.BaseURL,
		AuthScheme: dbCred.AuthScheme,
		ProviderID: dbCred.ProviderID,
		OrgID:      orgID,
	}, nil
}

// decryptWithDEKCache decrypts an API key using a DEK from the DEK cache
// (or falls back to KMS unwrap if the DEK isn't cached).
func (m *Manager) decryptWithDEKCache(ctx context.Context, credentialID string, encryptedKey, wrappedDEK []byte) ([]byte, error) {

	if enclave, ok := m.dekCache.Get(credentialID); ok {
		buf, err := enclave.Open()
		if err == nil {
			dek := make([]byte, buf.Size())
			copy(dek, buf.Bytes())
			buf.Destroy()

			apiKey, err := crypto.DecryptCredential(encryptedKey, dek)
			for i := range dek {
				dek[i] = 0
			}
			if err == nil {
				return apiKey, nil
			}
		}

		m.dekCache.Invalidate(credentialID)
	}

	dek, err := m.kms.Unwrap(ctx, wrappedDEK)
	if err != nil {
		return nil, fmt.Errorf("kms unwrap: %w", err)
	}

	apiKey, err := crypto.DecryptCredential(encryptedKey, dek)
	if err != nil {
		for i := range dek {
			dek[i] = 0
		}
		return nil, fmt.Errorf("decrypt: %w", err)
	}

	dekEnclave := memguard.NewEnclave(dek)
	m.dekCache.Set(credentialID, dekEnclave)
	for i := range dek {
		dek[i] = 0
	}

	return apiKey, nil
}

// promoteToL1 seals a copy of the plaintext API key in memguard and stores it in L1.
// NOTE: memguard.NewEnclave zeroes the source slice, so we copy first.
func (m *Manager) promoteToL1(credentialID string, orgID uuid.UUID, apiKey []byte, baseURL, authScheme, providerID string) {
	keyCopy := make([]byte, len(apiKey))
	copy(keyCopy, apiKey)
	enclave := memguard.NewEnclave(keyCopy)
	m.memory.Set(credentialID, &CachedCredential{
		Enclave:    enclave,
		BaseURL:    baseURL,
		AuthScheme: authScheme,
		ProviderID: providerID,
		OrgID:      orgID,
		CachedAt:   time.Now(),
		HardExpiry: time.Now().Add(m.hardExpiry),
	})
}
