package handler

import (
	"context"
	"fmt"

	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/model"
)

// decryptCredentialKey unwraps a credential's DEK and decrypts its API key.
// Shared by the platform-credential handlers (images, transcriptions).
func decryptCredentialKey(ctx context.Context, kms *crypto.KeyWrapper, cred *model.Credential) ([]byte, error) {
	dek, err := kms.Unwrap(ctx, cred.WrappedDEK)
	if err != nil {
		return nil, fmt.Errorf("kms unwrap: %w", err)
	}
	defer func() {
		for i := range dek {
			dek[i] = 0
		}
	}()
	apiKey, err := crypto.DecryptCredential(cred.EncryptedKey, dek)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}
	return apiKey, nil
}
