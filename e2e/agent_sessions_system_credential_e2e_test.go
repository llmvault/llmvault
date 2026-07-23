package e2e

import (
	"errors"
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

type agentSessionsSystemCredentialConfig struct {
	env        string
	label      string
	providerID string
	baseURL    string
	authScheme string
}

func agentSessionsEnsureSystemOpenRouterCredential(t *testing.T) {
	t.Helper()
	agentSessionsEnsureSystemCredential(t, agentSessionsSystemCredentialConfig{
		env:        "HIVY_SYSTEM_OPENROUTER_API_KEY",
		label:      "E2E System OpenRouter",
		providerID: "openrouter",
		baseURL:    "https://openrouter.ai/api/v1",
		authScheme: "bearer",
	})
}

func agentSessionsEnsureSystemTogetherCredential(t *testing.T) {
	t.Helper()
	agentSessionsEnsureSystemCredential(t, agentSessionsSystemCredentialConfig{
		env:        "HIVY_SYSTEM_TOGETHER_API_KEY",
		label:      "E2E System Together AI",
		providerID: "together",
		baseURL:    "https://api.together.ai/v1",
		authScheme: "bearer",
	})
}

func agentSessionsEnsureSystemReveCredential(t *testing.T) {
	t.Helper()
	agentSessionsEnsureSystemCredential(t, agentSessionsSystemCredentialConfig{
		env:        "HIVY_SYSTEM_REVE_API_KEY",
		label:      "E2E System Reve",
		providerID: "reve",
		baseURL:    "https://api.reve.com",
		authScheme: "bearer",
	})
}

func agentSessionsEnsureSystemQuiverCredential(t *testing.T) {
	t.Helper()
	agentSessionsEnsureSystemCredential(t, agentSessionsSystemCredentialConfig{
		env:        "HIVY_SYSTEM_QUIVER_API_KEY",
		label:      "E2E System Quiver",
		providerID: "quiver",
		baseURL:    "https://api.quiver.ai",
		authScheme: "bearer",
	})
}

func agentSessionsEnsureSystemElevenLabsCredential(t *testing.T) {
	t.Helper()
	agentSessionsEnsureSystemCredential(t, agentSessionsSystemCredentialConfig{
		env:        "HIVY_SYSTEM_ELEVENLABS_API_KEY",
		label:      "E2E System ElevenLabs",
		providerID: "elevenlabs",
		baseURL:    "https://api.elevenlabs.io",
		authScheme: "xi-api-key",
	})
}

func agentSessionsEnsureSystemCredential(t *testing.T, cfg agentSessionsSystemCredentialConfig) {
	t.Helper()
	loadEnv(t)

	apiKey := strings.TrimSpace(os.Getenv(cfg.env))
	if apiKey == "" {
		t.Fatalf("%s is required for %s", cfg.env, t.Name())
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
		var existing model.Credential
		err := tx.
			Where("org_id IS NULL AND revoked_at IS NULL AND provider_id = ?", cfg.providerID).
			Order("created_at ASC").
			First(&existing).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		cred, err := agentSessionsBuildSystemCredential(t, kms, cfg, apiKey)
		if err != nil {
			return err
		}
		if existing.ID != uuid.Nil {
			return tx.Model(&model.Credential{}).Where("id = ?", existing.ID).Updates(map[string]any{
				"label":         cfg.label,
				"base_url":      cfg.baseURL,
				"auth_scheme":   cfg.authScheme,
				"encrypted_key": cred.EncryptedKey,
				"wrapped_dek":   cred.WrappedDEK,
				"meta":          cred.Meta,
			}).Error
		}
		return tx.Create(&cred).Error
	})
	if err != nil {
		t.Fatalf("seed E2E system %s credential: %v", cfg.providerID, err)
	}
}

func agentSessionsBuildSystemCredential(t *testing.T, kms *crypto.KeyWrapper, cfg agentSessionsSystemCredentialConfig, apiKey string) (model.Credential, error) {
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
		Label:        cfg.label,
		BaseURL:      cfg.baseURL,
		AuthScheme:   cfg.authScheme,
		ProviderID:   cfg.providerID,
		EncryptedKey: encryptedKey,
		WrappedDEK:   wrappedDEK,
		Meta: model.JSON{
			"source": "e2e",
			"env":    cfg.env,
		},
		CreatedAt: time.Now(),
	}, nil
}

func agentSessionsZeroBytes(buf []byte) {
	for i := range buf {
		buf[i] = 0
	}
}
