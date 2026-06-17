package e2e

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/model"
)

const agentSessionsSystemCredentialSeedLockKey int64 = 2026061702

func agentSessionsEnsureSystemOpenRouterCredential(t *testing.T) {
	t.Helper()
	loadEnv(t)

	apiKey := strings.TrimSpace(os.Getenv("HIVY_SYSTEM_OPENROUTER_API_KEY"))
	if apiKey == "" {
		t.Fatalf("HIVY_SYSTEM_OPENROUTER_API_KEY is required for %s", t.Name())
	}
	kmsType := strings.TrimSpace(os.Getenv("HIVY_KMS_TYPE"))
	if kmsType == "" {
		kmsType = "aead"
	}
	if kmsType != "aead" {
		t.Fatalf("%s only supports HIVY_KMS_TYPE=aead for local E2E credential seeding; got %q", t.Name(), kmsType)
	}
	kmsKey := strings.TrimSpace(os.Getenv("HIVY_KMS_KEY"))
	if kmsKey == "" {
		t.Fatalf("HIVY_KMS_KEY is required for %s", t.Name())
	}
	kms, err := crypto.NewAEADWrapper(t.Context(), kmsKey, "aead-local")
	if err != nil {
		t.Fatalf("create E2E KMS wrapper: %v", err)
	}

	db := agentSessionsOpenDB(t)
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", agentSessionsSystemCredentialSeedLockKey).Error; err != nil {
			return err
		}
		var existing int64
		if err := tx.Model(&model.Credential{}).
			Where("org_id IS NULL AND revoked_at IS NULL AND provider_id = ?", "openrouter").
			Count(&existing).Error; err != nil {
			return err
		}
		if existing > 0 {
			return nil
		}
		cred, err := agentSessionsBuildSystemOpenRouterCredential(t, kms, apiKey)
		if err != nil {
			return err
		}
		return tx.Create(&cred).Error
	})
	if err != nil {
		t.Fatalf("seed E2E system OpenRouter credential: %v", err)
	}
}

func agentSessionsBuildSystemOpenRouterCredential(t *testing.T, kms *crypto.KeyWrapper, apiKey string) (model.Credential, error) {
	t.Helper()
	dek, err := crypto.GenerateDEK()
	if err != nil {
		return model.Credential{}, err
	}
	defer agentSessionsZeroBytes(dek)

	encryptedKey, err := crypto.EncryptCredential([]byte(apiKey), dek)
	if err != nil {
		return model.Credential{}, err
	}
	wrappedDEK, err := kms.Wrap(t.Context(), dek)
	if err != nil {
		return model.Credential{}, err
	}
	return model.Credential{
		ID:           uuid.New(),
		Label:        "E2E System OpenRouter",
		BaseURL:      "https://openrouter.ai/api/v1",
		AuthScheme:   "bearer",
		ProviderID:   "openrouter",
		EncryptedKey: encryptedKey,
		WrappedDEK:   wrappedDEK,
		Meta: model.JSON{
			"source": "e2e",
			"env":    "HIVY_SYSTEM_OPENROUTER_API_KEY",
		},
		CreatedAt: time.Now(),
	}, nil
}

func agentSessionsZeroBytes(buf []byte) {
	for i := range buf {
		buf[i] = 0
	}
}
