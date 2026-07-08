package handler_test

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/model"
)

// Shared test helpers for handlers that decrypt credential API keys (image and
// transcription handlers). Previously lived in the now-removed system-tasks test
// setup file.

func sysShortID() string { return uuid.New().String()[:8] }

func tptr(t time.Time) *time.Time { return &t }

func newSystemTaskKMS(t *testing.T) *crypto.KeyWrapper {
	t.Helper()
	key := make([]byte, 32)
	b64 := base64.StdEncoding.EncodeToString(key)
	kms, err := crypto.NewAEADWrapper(context.Background(), b64, "credential-test")
	if err != nil {
		t.Fatalf("KMS: %v", err)
	}
	return kms
}

func seedSystemCredential(t *testing.T, db *gorm.DB, kms *crypto.KeyWrapper, baseURL, providerID string) *model.Credential {
	t.Helper()
	dek, err := crypto.GenerateDEK()
	if err != nil {
		t.Fatalf("dek: %v", err)
	}
	encKey, err := crypto.EncryptCredential([]byte("sk-fake"), dek)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	wrapped, err := kms.Wrap(context.Background(), dek)
	if err != nil {
		t.Fatalf("wrap: %v", err)
	}
	for i := range dek {
		dek[i] = 0
	}
	cred := &model.Credential{
		Label:        "test-system-" + sysShortID(),
		BaseURL:      baseURL,
		AuthScheme:   "bearer",
		ProviderID:   providerID,
		EncryptedKey: encKey,
		WrappedDEK:   wrapped,
	}
	if err := db.Create(cred).Error; err != nil {
		t.Fatalf("create system credential: %v", err)
	}
	return cred
}
