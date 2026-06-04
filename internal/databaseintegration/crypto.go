package databaseintegration

import (
	"context"
	"fmt"

	"github.com/usehivy/hivy/internal/crypto"
)

func EncryptSecret(ctx context.Context, kms *crypto.KeyWrapper, value string) ([]byte, []byte, error) {
	dek, err := crypto.GenerateDEK()
	if err != nil {
		return nil, nil, fmt.Errorf("generate dek: %w", err)
	}
	encrypted, err := crypto.EncryptCredential([]byte(value), dek)
	if err != nil {
		return nil, nil, fmt.Errorf("encrypt secret: %w", err)
	}
	wrapped, err := kms.Wrap(ctx, dek)
	if err != nil {
		return nil, nil, fmt.Errorf("wrap dek: %w", err)
	}
	for i := range dek {
		dek[i] = 0
	}
	return encrypted, wrapped, nil
}

func DecryptSecret(ctx context.Context, kms *crypto.KeyWrapper, encrypted, wrapped []byte) (string, error) {
	dek, err := kms.Unwrap(ctx, wrapped)
	if err != nil {
		return "", fmt.Errorf("unwrap dek: %w", err)
	}
	plain, err := crypto.DecryptCredential(encrypted, dek)
	if err != nil {
		return "", fmt.Errorf("decrypt secret: %w", err)
	}
	for i := range dek {
		dek[i] = 0
	}
	return string(plain), nil
}
