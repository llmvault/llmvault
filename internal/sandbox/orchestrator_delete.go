package sandbox

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

func (o *Orchestrator) StopSandbox(ctx context.Context, sb *model.Sandbox) error {
	if err := o.ensureSandboxProvider(sb); err != nil {
		return err
	}

	unlock := o.lifecycle.Lock(sb.ID.String())
	defer unlock()

	var fresh model.Sandbox
	if err := o.db.First(&fresh, "id = ?", sb.ID).Error; err == nil {
		sb = &fresh
	}
	if sb.Status == string(StatusStopped) {
		return nil
	}
	if err := o.provider.StopSandbox(ctx, sb.ExternalID); err != nil {
		if errors.Is(err, ErrSandboxNotFound) {
			return o.purgeMissingSandbox(sb)
		}
		if errors.Is(err, ErrUnsupported) {
			// No pause primitive (Railway): do NOT persist 'stopped' (still running
			// and billing). Surface the sentinel so callers skip or fall back to delete.
			logging.FromContext(ctx).InfoContext(ctx, "stop sandbox unsupported by provider; leaving running",
				"sandbox_id", sb.ID, "provider", o.providerID())
			return err
		}
		return fmt.Errorf("stopping sandbox %s: %w", sb.ID, err)
	}
	now := time.Now()
	if err := o.db.Model(sb).Updates(map[string]any{
		"status":                 "stopped",
		"stopped_at":             now,
		"runtime_url_expires_at": nil,
	}).Error; err != nil {
		return err
	}
	sb.Status = "stopped"
	sb.StoppedAt = &now
	sb.RuntimeURLExpiresAt = nil
	return nil
}

func (o *Orchestrator) DeleteSandbox(ctx context.Context, sb *model.Sandbox) error {
	if err := o.ensureSandboxProvider(sb); err != nil {
		return err
	}
	unlock := o.lifecycle.Lock(sb.ID.String())
	defer unlock()

	if err := o.provider.DeleteSandbox(ctx, sb.ExternalID); err != nil && !errors.Is(err, ErrSandboxNotFound) {
		logging.Capture(ctx, fmt.Errorf("delete sandbox %s from provider: %w", sb.ID, err))
	}
	return o.db.Where("id = ?", sb.ID).Delete(&model.Sandbox{}).Error
}

// DeleteSandboxResource deletes the provider resource but keeps the control
// plane sandbox row for task/session history that points at the sandbox.
func (o *Orchestrator) DeleteSandboxResource(ctx context.Context, sb *model.Sandbox) error {
	if err := o.ensureSandboxProvider(sb); err != nil {
		return err
	}
	unlock := o.lifecycle.Lock(sb.ID.String())
	defer unlock()

	if err := o.provider.DeleteSandbox(ctx, sb.ExternalID); err != nil && !errors.Is(err, ErrSandboxNotFound) {
		logging.Capture(ctx, fmt.Errorf("delete sandbox %s from provider: %w", sb.ID, err))
		return fmt.Errorf("delete sandbox resource %s: %w", sb.ID, err)
	}
	now := time.Now()
	if err := o.db.Model(sb).Updates(map[string]any{
		"status":                 string(StatusArchived),
		"stopped_at":             now,
		"runtime_url_expires_at": nil,
	}).Error; err != nil {
		return fmt.Errorf("mark sandbox resource deleted: %w", err)
	}
	sb.Status = string(StatusArchived)
	sb.StoppedAt = &now
	sb.RuntimeURLExpiresAt = nil
	return nil
}

func (o *Orchestrator) DeleteSandboxExternal(ctx context.Context, externalID string) error {
	if err := o.provider.DeleteSandbox(ctx, externalID); err != nil && !errors.Is(err, ErrSandboxNotFound) {
		return fmt.Errorf("delete sandbox %s from provider: %w", externalID, err)
	}
	return nil
}

func (o *Orchestrator) ArchiveSandbox(ctx context.Context, sb *model.Sandbox) error {
	if err := o.ensureSandboxProvider(sb); err != nil {
		return err
	}
	if sb.Status != string(StatusStopped) {
		if err := o.StopSandbox(ctx, sb); err != nil {
			if errors.Is(err, ErrSandboxNotFound) {
				return nil
			}
			if errors.Is(err, ErrUnsupported) {
				// Provider can't stop (Railway): delete the live resource instead of
				// persisting an 'archived' lie that keeps billing.
				logging.FromContext(ctx).InfoContext(ctx, "archive sandbox unsupported by provider; deleting resource",
					"sandbox_id", sb.ID, "provider", o.providerID())
				return o.DeleteSandboxResource(ctx, sb)
			}
			return fmt.Errorf("stopping sandbox before archive: %w", err)
		}
	}

	if err := o.provider.ArchiveSandbox(ctx, sb.ExternalID); err != nil {
		if errors.Is(err, ErrSandboxNotFound) {
			return o.purgeMissingSandbox(sb)
		}
		if errors.Is(err, ErrUnsupported) {
			logging.FromContext(ctx).InfoContext(ctx, "archive sandbox unsupported by provider; deleting resource",
				"sandbox_id", sb.ID, "provider", o.providerID())
			return o.DeleteSandboxResource(ctx, sb)
		}
		return fmt.Errorf("archiving sandbox %s: %w", sb.ID, err)
	}

	if err := o.db.Model(sb).Updates(map[string]any{
		"status":                 string(StatusArchived),
		"runtime_url_expires_at": nil,
	}).Error; err != nil {
		return fmt.Errorf("marking sandbox archived: %w", err)
	}
	sb.Status = string(StatusArchived)
	sb.RuntimeURLExpiresAt = nil

	logging.FromContext(ctx).InfoContext(ctx, "sandbox archived", "sandbox_id", sb.ID, "external_id", sb.ExternalID)
	return nil
}

func (o *Orchestrator) purgeMissingSandbox(sb *model.Sandbox) error {
	if err := o.db.Where("id = ?", sb.ID).Delete(&model.Sandbox{}).Error; err != nil {
		return fmt.Errorf("purging missing sandbox %s: %w", sb.ID, err)
	}
	return ErrSandboxNotFound
}
