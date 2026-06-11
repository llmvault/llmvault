package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/sandbox"
)

// buildSandboxOrchestrator wires the sandbox encryption key and orchestrator.
// Both are nil (orchestration disabled) when no provider is configured, the
// encryption key is missing, or the provider is incomplete/unavailable outside
// production. A configured-but-failing provider is a hard error in production
// (P1-20).
func buildSandboxOrchestrator(ctx context.Context, cfg *config.Config, database *gorm.DB) (*crypto.SymmetricKey, *sandbox.Orchestrator, error) {
	if cfg.SandboxProviderID == "" {
		logging.FromContext(ctx).WarnContext(ctx, "sandbox orchestration disabled; no sandbox provider configured",
			"required_env", "HIVY_SANDBOX_PROVIDER_ID,HIVY_SANDBOX_ENCRYPTION_KEY")
		return nil, nil, nil
	}
	if cfg.SandboxEncryptionKey == "" {
		logging.FromContext(ctx).WarnContext(ctx, "sandbox orchestration disabled; sandbox provider configured without encryption key",
			"provider", cfg.SandboxProviderID,
			"missing_env", "HIVY_SANDBOX_ENCRYPTION_KEY")
		return nil, nil, nil
	}

	sandboxEncKey, err := crypto.NewSymmetricKey(cfg.SandboxEncryptionKey)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid HIVY_SANDBOX_ENCRYPTION_KEY: %w", err)
	}

	sandboxProvider, err := newSandboxProvider(cfg)
	if err != nil {
		if errors.Is(err, errSandboxProviderNotConfigured) {
			logging.FromContext(ctx).WarnContext(ctx, "sandbox orchestration disabled; selected sandbox provider is incomplete",
				"provider", cfg.SandboxProviderID,
				"reason", err.Error())
			return sandboxEncKey, nil, nil
		}
		return nil, nil, fmt.Errorf("creating sandbox provider: %w", err)
	}
	if err := validateSandboxProvider(ctx, cfg, sandboxProvider); err != nil {
		// A configured sandbox provider that fails validation is a hard error
		// in production: a flaky provider response at boot must not yield a
		// "healthy" instance with the entire employee/gateway subsystem silently
		// missing (P1-20). validateSandboxProvider already retried with backoff.
		if cfg.IsProduction() {
			return nil, nil, fmt.Errorf("validating sandbox provider %q: %w", sandboxProvider.ID(), err)
		}
		logging.FromContext(ctx).WarnContext(ctx, "sandbox orchestration disabled; selected sandbox provider is unavailable",
			"provider", sandboxProvider.ID(),
			"reason", err.Error())
		return sandboxEncKey, nil, nil
	}

	orchestrator := sandbox.NewOrchestrator(database, sandboxProvider, sandboxEncKey, cfg)
	logging.FromContext(ctx).InfoContext(ctx, "sandbox orchestrator ready", "provider", sandboxProvider.ID())
	return sandboxEncKey, orchestrator, nil
}

// validateSandboxProvider validates the provider, retrying transient failures
// with a short backoff so a flaky provider response at deploy time does not
// permanently disable sandbox orchestration for the process lifetime (P1-20).
// In production the number of attempts is higher and the final error is
// returned to the caller, which fails bootstrap.
func validateSandboxProvider(ctx context.Context, cfg *config.Config, provider sandbox.Provider) error {
	attempts := 3
	if cfg.IsProduction() {
		attempts = 6
	}
	var err error
	for attempt := 1; attempt <= attempts; attempt++ {
		if err = provider.Validate(ctx); err == nil {
			return nil
		}
		if attempt == attempts {
			break
		}
		backoff := time.Duration(attempt) * 2 * time.Second
		logging.FromContext(ctx).WarnContext(ctx, "sandbox provider validation failed; retrying",
			"provider", provider.ID(),
			"attempt", attempt,
			"max_attempts", attempts,
			"retry_in", backoff.String(),
			"error", err.Error())
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
	}
	return err
}
