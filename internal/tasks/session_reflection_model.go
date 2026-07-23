package tasks

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/cache"
	"github.com/usehivy/hivy/internal/credentials"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/registry"
)

// Session reflection gets its own provider/model/temperature resolution,
// separate from session naming. Every knob is env-overridable:
//
//	HIVY_REFLECTION_PROVIDER     provider of the org-wide system credential (default "openai")
//	HIVY_REFLECTION_MODEL       canonical Hivy model ID or provider upstream ID (default "gpt-4o-mini")
//	HIVY_REFLECTION_TEMPERATURE sampling temperature (default 0.1)
const (
	reflectionProviderEnv    = "HIVY_REFLECTION_PROVIDER"
	reflectionModelEnv       = "HIVY_REFLECTION_MODEL"
	reflectionTemperatureEnv = "HIVY_REFLECTION_TEMPERATURE"

	reflectionDefaultProviderID  = "openai"
	reflectionDefaultModelID     = "gpt-4o-mini"
	reflectionDefaultTemperature = 0.1
)

type reflectionModelConfig struct {
	ProviderID  string
	ModelID     string
	Temperature float64
}

// reflectionCredential is the resolved system credential + upstream model for
// a reflection run. Temperature is resolved here and forwarded to the
// provider via hivy.CompletionRequest.Temperature.
type reflectionCredential struct {
	credential  *model.Credential
	apiKey      string
	modelID     string
	temperature float64
}

type reflectionCredentialLoader func(
	ctx context.Context,
	db *gorm.DB,
	cacheManager *cache.Manager,
	reg *registry.Registry,
) (*reflectionCredential, error)

func reflectionModelConfigFromEnv() reflectionModelConfig {
	cfg := reflectionModelConfig{
		ProviderID:  reflectionDefaultProviderID,
		ModelID:     reflectionDefaultModelID,
		Temperature: reflectionDefaultTemperature,
	}
	if value := strings.TrimSpace(os.Getenv(reflectionProviderEnv)); value != "" {
		cfg.ProviderID = value
	}
	if value := strings.TrimSpace(os.Getenv(reflectionModelEnv)); value != "" {
		cfg.ModelID = value
	}
	if value := strings.TrimSpace(os.Getenv(reflectionTemperatureEnv)); value != "" {
		if parsed, err := strconv.ParseFloat(value, 64); err == nil && parsed >= 0 && parsed <= 2 {
			cfg.Temperature = parsed
		}
	}
	return cfg
}

// ResolveReflectionModel returns the provider, upstream model ID, and
// temperature the reflection pipeline will use, applying env overrides.
// Exported for the replay harness so CLI and production resolve identically.
func ResolveReflectionModel(reg *registry.Registry) (providerID, upstreamModelID string, temperature float64) {
	cfg := reflectionModelConfigFromEnv()
	return cfg.ProviderID, resolveReflectionUpstreamModel(reg, cfg.ProviderID, cfg.ModelID), cfg.Temperature
}

// resolveReflectionUpstreamModel maps the configured model to the provider's
// upstream ID. Canonical Hivy model IDs resolve through the registry; the
// Anything not in the curated canonical catalog is passed through verbatim so
// env overrides can name a provider upstream model directly.
func resolveReflectionUpstreamModel(reg *registry.Registry, providerID, modelID string) string {
	if reg == nil {
		reg = registry.Global()
	}
	if route, ok := reg.ResolveModel(providerID, modelID); ok {
		return route.UpstreamID
	}
	return modelID
}

func loadReflectionCredential(
	ctx context.Context,
	db *gorm.DB,
	cacheManager *cache.Manager,
	reg *registry.Registry,
) (*reflectionCredential, error) {
	if db == nil || cacheManager == nil {
		return nil, fmt.Errorf("missing reflection dependencies")
	}
	if reg == nil {
		reg = registry.Global()
	}
	cfg := reflectionModelConfigFromEnv()
	upstreamModelID := resolveReflectionUpstreamModel(reg, cfg.ProviderID, cfg.ModelID)

	credential, err := pickReflectionCredential(ctx, db, cfg)
	if err != nil {
		return nil, err
	}
	decrypted, err := cacheManager.GetDecryptedCredentialByID(ctx, credential.ID.String())
	if err != nil {
		return nil, fmt.Errorf("decrypt reflection system credential: %w", err)
	}
	return &reflectionCredential{
		credential:  credential,
		apiKey:      string(decrypted.APIKey),
		modelID:     upstreamModelID,
		temperature: cfg.Temperature,
	}, nil
}

func pickReflectionCredential(ctx context.Context, db *gorm.DB, cfg reflectionModelConfig) (*model.Credential, error) {
	var creds []model.Credential
	if err := db.WithContext(ctx).
		Where("org_id IS NULL AND revoked_at IS NULL AND provider_id = ?", cfg.ProviderID).
		Order("created_at ASC").
		Find(&creds).Error; err != nil {
		return nil, fmt.Errorf("list active reflection credentials: %w", err)
	}
	if len(creds) == 0 {
		return nil, fmt.Errorf(
			"%w: provider=%q model=%q",
			credentials.ErrNoSystemCredential,
			cfg.ProviderID,
			cfg.ModelID,
		)
	}
	return &creds[0], nil
}
