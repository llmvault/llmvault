package tasks

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sandbox"
)

// TokenCleanupHandler deletes expired email verifications, password resets, and OAuth exchange tokens.
type TokenCleanupHandler struct {
	db *gorm.DB
}

func NewTokenCleanupHandler(db *gorm.DB) *TokenCleanupHandler {
	return &TokenCleanupHandler{db: db}
}

func (h *TokenCleanupHandler) Handle(ctx context.Context, _ *asynq.Task) error {
	cutoff := time.Now().Add(-7 * 24 * time.Hour)
	del := func(label string, dst any) error {
		if err := h.db.WithContext(ctx).
			Where("expires_at < ? OR used_at < ?", cutoff, cutoff).
			Delete(dst).Error; err != nil {
			return fmt.Errorf("%s: %w", label, err)
		}
		return nil
	}
	err := errors.Join(
		del("email_verifications", &model.EmailVerification{}),
		del("password_resets", &model.PasswordReset{}),
		del("oauth_exchange_tokens", &model.OAuthExchangeToken{}),
	)
	if err != nil {
		// Returning the error lets asynq retry the cleanup; a transient DB blip
		// must not silently leave expired tokens around.
		logging.FromContext(ctx).ErrorContext(ctx, "token cleanup failed", "error", err)
		return err
	}
	logging.FromContext(ctx).DebugContext(ctx, "cleaned up expired verification/reset tokens")
	return nil
}

// SandboxResourceCheckHandler collects cgroup resource stats from running sandboxes.
type SandboxResourceCheckHandler struct {
	orchestrator *sandbox.Orchestrator
}

func NewSandboxResourceCheckHandler(orchestrator *sandbox.Orchestrator) *SandboxResourceCheckHandler {
	return &SandboxResourceCheckHandler{orchestrator: orchestrator}
}

func (h *SandboxResourceCheckHandler) Handle(ctx context.Context, _ *asynq.Task) error {
	h.orchestrator.RunResourceCheck(ctx)
	return nil
}

// SandboxReapHandler releases leaked paid compute the inline cleanup missed
// (worker death mid-provision and stranded warm slots).
type SandboxReapHandler struct {
	orchestrator *sandbox.Orchestrator
}

func NewSandboxReapHandler(orchestrator *sandbox.Orchestrator) *SandboxReapHandler {
	return &SandboxReapHandler{orchestrator: orchestrator}
}

func (h *SandboxReapHandler) Handle(ctx context.Context, _ *asynq.Task) error {
	h.orchestrator.RunSandboxReaper(ctx)
	return nil
}
