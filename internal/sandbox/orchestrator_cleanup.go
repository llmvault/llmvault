package sandbox

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

// cleanupFailedSandbox is the single shared post-create failure handler. Any
// failure after a provider resource has been created (or warm-claimed) must run
// through this so the paid compute is released and the minted proxy token is
// revoked instead of leaking a live sandbox with valid credentials baked in.
//
// It records the failure on the row (status=error + message), deletes the
// provider resource, and revokes every proxy token minted for the sandbox. The
// provider delete and token revoke run on context.WithoutCancel so they still
// execute when the originating request context is already cancelled (the common
// case when a sync times out mid-provision).
func (o *Orchestrator) cleanupFailedSandbox(ctx context.Context, sb *model.Sandbox, externalID, message string) {
	if sb == nil {
		return
	}
	cleanupCtx := context.WithoutCancel(ctx)

	updates := map[string]any{
		"status":        string(StatusError),
		"error_message": message,
	}
	if externalID != "" {
		updates["external_id"] = externalID
	}
	if err := o.db.WithContext(cleanupCtx).Model(sb).Updates(updates).Error; err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "cleanup failed sandbox: mark error",
			"error", err, "sandbox_id", sb.ID)
	}

	id := externalID
	if id == "" {
		id = sb.ExternalID
	}
	o.deleteProviderResource(cleanupCtx, sb.ID, id)
	o.revokeSandboxProxyTokens(cleanupCtx, sb.ID)
}

// deleteProviderResource deletes the provider-side sandbox compute. Missing
// resources are treated as success. Errors are captured but not surfaced — the
// caller is already returning a failure and the stuck-creating/error reaper is
// the backstop for a transient provider error here.
func (o *Orchestrator) deleteProviderResource(ctx context.Context, sandboxID uuid.UUID, externalID string) {
	if externalID == "" {
		return
	}
	if err := o.provider.DeleteSandbox(ctx, externalID); err != nil && !errors.Is(err, ErrSandboxNotFound) {
		logging.Capture(ctx, fmt.Errorf("cleanup failed sandbox %s: delete provider resource %s: %w", sandboxID, externalID, err))
	}
}

// revokeSandboxProxyTokens revokes every non-revoked proxy token minted for the
// sandbox so a leaked or to-be-deleted sandbox cannot keep using its LLM/MCP
// credentials.
func (o *Orchestrator) revokeSandboxProxyTokens(ctx context.Context, sandboxID uuid.UUID) {
	now := time.Now()
	if err := o.db.WithContext(ctx).Model(&model.Token{}).
		Where("meta ->> ? = ? AND revoked_at IS NULL", model.TokenMetaSandboxID, sandboxID.String()).
		Update("revoked_at", &now).Error; err != nil {
		logging.Capture(ctx, fmt.Errorf("cleanup failed sandbox %s: revoke proxy tokens: %w", sandboxID, err))
	}
}

// RunSandboxReaper releases leaked paid compute: stuck creating/error
// sandboxes, idle/terminated specialist sandboxes, and stranded warm slots.
func (o *Orchestrator) RunSandboxReaper(ctx context.Context) {
	o.ReapStuckSandboxes(ctx)
	if o.warmPool != nil {
		if err := o.warmPool.ReapStaleSlots(ctx); err != nil {
			logging.FromContext(ctx).ErrorContext(ctx, "sandbox reaper: reap stale warm slots failed", "error", err)
		}
	}
}

// ReapStuckSandboxes deletes provider resources for sandboxes left in
// 'creating' or 'error' beyond a TTL. Failed provisioning paths run
// cleanupFailedSandbox inline, but a worker death mid-create (or a transient
// provider error during that cleanup) can still strand a live, billing sandbox;
// this is the backstop. It also reaps idle/terminated specialist sandboxes that
// the LLM never explicitly terminated.
func (o *Orchestrator) ReapStuckSandboxes(ctx context.Context) {
	const (
		creatingTTL = 30 * time.Minute
		errorTTL    = 15 * time.Minute
	)
	now := time.Now()

	o.reapSandboxesByStatus(ctx, string(StatusCreating), now.Add(-creatingTTL))
	o.reapSandboxesByStatus(ctx, string(StatusError), now.Add(-errorTTL))
	o.reapIdleSpecialistSandboxes(ctx, now)
}

func (o *Orchestrator) reapSandboxesByStatus(ctx context.Context, status string, cutoff time.Time) {
	var stuck []model.Sandbox
	if err := o.db.WithContext(ctx).
		Where("status = ? AND provider_id = ? AND updated_at < ?", status, o.providerID(), cutoff).
		Find(&stuck).Error; err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "reap stuck sandboxes: query failed",
			"error", err, "status", status)
		return
	}
	for i := range stuck {
		sb := &stuck[i]
		o.deleteProviderResource(ctx, sb.ID, sb.ExternalID)
		o.revokeSandboxProxyTokens(ctx, sb.ID)
		if err := o.db.WithContext(ctx).Model(sb).Updates(map[string]any{
			"status":                 string(StatusArchived),
			"runtime_url_expires_at": nil,
		}).Error; err != nil {
			logging.FromContext(ctx).WarnContext(ctx, "reap stuck sandboxes: mark archived failed",
				"sandbox_id", sb.ID, "error", err)
			continue
		}
		logging.FromContext(ctx).InfoContext(ctx, "reaped stuck sandbox",
			"sandbox_id", sb.ID, "external_id", sb.ExternalID, "from_status", status)
	}
}

// reapIdleSpecialistSandboxes deletes the provider resource for specialist
// sandboxes whose task is idle/terminated/error beyond a TTL. Normal specialist
// task completion only marks the task idle; without this the sandbox runs and
// bills forever.
func (o *Orchestrator) reapIdleSpecialistSandboxes(ctx context.Context, now time.Time) {
	const idleSpecialistTTL = 1 * time.Hour
	cutoff := now.Add(-idleSpecialistTTL)

	var sandboxes []model.Sandbox
	if err := o.db.WithContext(ctx).
		Where(`status = ? AND provider_id = ? AND id IN (
			SELECT sandbox_id FROM specialist_tasks
			WHERE status IN ('idle', 'terminated', 'error') AND updated_at < ?
		)`, string(StatusRunning), o.providerID(), cutoff).
		Find(&sandboxes).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return
		}
		logging.FromContext(ctx).ErrorContext(ctx, "reap idle specialist sandboxes: query failed", "error", err)
		return
	}
	for i := range sandboxes {
		sb := &sandboxes[i]
		if err := o.DeleteSandboxResource(ctx, sb); err != nil {
			logging.FromContext(ctx).WarnContext(ctx, "reap idle specialist sandboxes: delete resource failed",
				"sandbox_id", sb.ID, "error", err)
			continue
		}
		o.revokeSandboxProxyTokens(ctx, sb.ID)
		logging.FromContext(ctx).InfoContext(ctx, "reaped idle specialist sandbox",
			"sandbox_id", sb.ID, "external_id", sb.ExternalID)
	}
}
