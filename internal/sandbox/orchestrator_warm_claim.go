package sandbox

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

func (o *Orchestrator) tryClaimWarmRuntime(ctx context.Context, sb *model.Sandbox, profile WarmPoolProfile) (bool, error) {
	if o.warmPool == nil {
		return false, nil
	}
	claimed, err := o.warmPool.Claim(ctx, profile, sb.ID)
	if err != nil {
		if isNoWarmSlotAvailable(err) {
			o.enqueueWarmPoolReconcile(ctx, profile)
			return false, nil
		}
		return false, fmt.Errorf("claim warm runtime: %w", err)
	}

	encryptedRuntimeSecret, err := o.encKey.EncryptString(claimed.RuntimeSecret)
	if err != nil {
		_ = o.warmPool.MarkError(context.WithoutCancel(ctx), claimed.ID, fmt.Sprintf("encrypt runtime secret: %v", err))
		return false, fmt.Errorf("encrypt claimed runtime secret: %w", err)
	}

	now := time.Now()
	expiresAt := now.Add(runtimeURLTTL)
	if err := o.db.Model(sb).Updates(map[string]any{
		"external_id":              claimed.ExternalID,
		"runtime_url":              claimed.EndpointURL,
		"runtime_url_expires_at":   expiresAt,
		"encrypted_runtime_secret": encryptedRuntimeSecret,
		"status":                   "running",
		"last_active_at":           now,
	}).Error; err != nil {
		_ = o.warmPool.MarkError(context.WithoutCancel(ctx), claimed.ID, fmt.Sprintf("update sandbox: %v", err))
		return false, fmt.Errorf("updating claimed sandbox: %w", err)
	}
	sb.ExternalID = claimed.ExternalID
	sb.RuntimeURL = claimed.EndpointURL
	sb.RuntimeURLExpiresAt = &expiresAt
	sb.EncryptedRuntimeSecret = encryptedRuntimeSecret
	sb.Status = "running"
	sb.LastActiveAt = &now

	if err := o.waitForAgentRuntimeLive(ctx, sb); err != nil {
		_ = o.warmPool.MarkError(context.WithoutCancel(ctx), claimed.ID, fmt.Sprintf("runtime health: %v", err))
		return false, fmt.Errorf("waiting for claimed runtime: %w", err)
	}
	if profile.Mode == model.SandboxWarmSlotModeAgent {
		if err := o.pushAgentRuntimeConfig(ctx, sb, "warm claim"); err != nil {
			_ = o.warmPool.MarkError(context.WithoutCancel(ctx), claimed.ID, fmt.Sprintf("runtime config push: %v", err))
			return false, err
		}
	}
	if err := o.warmPool.MarkClaimed(ctx, claimed.ID); err != nil {
		return false, fmt.Errorf("mark warm slot claimed: %w", err)
	}
	o.enqueueWarmPoolReconcile(ctx, profile)
	return true, nil
}

func isNoWarmSlotAvailable(err error) bool {
	if err == nil {
		return false
	}
	return err == gorm.ErrRecordNotFound ||
		(strings.Contains(err.Error(), "no warm ") && strings.Contains(err.Error(), "sandbox slots available"))
}
