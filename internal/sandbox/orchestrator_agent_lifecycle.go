package sandbox

import (
	"context"
	"fmt"
	"time"

	"github.com/usehivy/hivy/internal/model"
)

func (o *Orchestrator) RefreshAgentSandboxURL(ctx context.Context, sb *model.Sandbox) error {
	if err := o.ensureSandboxProvider(sb); err != nil {
		return err
	}
	url, err := o.provider.GetEndpoint(ctx, sb.ExternalID, AgentSandboxPort)
	if err != nil {
		return fmt.Errorf("get agent sandbox endpoint: %w", err)
	}
	expiresAt := time.Now().Add(runtimeURLTTL)
	if err := o.db.Model(sb).Updates(map[string]any{
		"runtime_url":            url,
		"runtime_url_expires_at": expiresAt,
	}).Error; err != nil {
		return fmt.Errorf("update sandbox url: %w", err)
	}
	sb.RuntimeURL = url
	sb.RuntimeURLExpiresAt = &expiresAt
	return nil
}

func (o *Orchestrator) StartAgentSandbox(ctx context.Context, sb *model.Sandbox) error {
	if err := o.ensureSandboxProvider(sb); err != nil {
		return err
	}

	unlock := o.lifecycle.Lock(sb.ID.String())
	defer unlock()

	var fresh model.Sandbox
	if err := o.db.First(&fresh, "id = ?", sb.ID).Error; err == nil {
		sb = &fresh
	}
	if sb.Status != string(StatusRunning) {
		if err := o.provider.StartSandbox(ctx, sb.ExternalID); err != nil {
			return fmt.Errorf("starting agent sandbox %s: %w", sb.ID, err)
		}
	}
	if err := o.RefreshAgentSandboxURL(ctx, sb); err != nil {
		return err
	}
	if err := o.waitForAgentRuntimeLive(ctx, sb); err != nil {
		return fmt.Errorf("waiting for agent runtime: %w", err)
	}
	now := time.Now()
	if err := o.db.Model(sb).Updates(map[string]any{
		"status":         string(StatusRunning),
		"last_active_at": now,
		"stopped_at":     nil,
		"error_message":  nil,
	}).Error; err != nil {
		return fmt.Errorf("mark agent sandbox running: %w", err)
	}
	sb.Status = string(StatusRunning)
	sb.LastActiveAt = &now
	sb.StoppedAt = nil
	sb.ErrorMessage = nil
	if err := o.pushAgentRuntimeConfig(ctx, sb, "start"); err != nil {
		return err
	}
	return nil
}

func (o *Orchestrator) RestartAgentSandbox(ctx context.Context, sb *model.Sandbox) error {
	if err := o.ensureSandboxProvider(sb); err != nil {
		return err
	}
	if restarter, ok := o.provider.(RestartableProvider); ok {
		unlock := o.lifecycle.Lock(sb.ID.String())
		defer unlock()

		if err := restarter.RestartSandbox(ctx, sb.ExternalID); err != nil {
			return fmt.Errorf("restarting agent sandbox %s: %w", sb.ID, err)
		}
		if err := o.RefreshAgentSandboxURL(ctx, sb); err != nil {
			return err
		}
		if err := o.waitForAgentRuntimeLive(ctx, sb); err != nil {
			return fmt.Errorf("waiting for agent runtime: %w", err)
		}
		now := time.Now()
		if err := o.db.Model(sb).Updates(map[string]any{
			"status":         string(StatusRunning),
			"last_active_at": now,
			"stopped_at":     nil,
			"error_message":  nil,
		}).Error; err != nil {
			return fmt.Errorf("mark agent sandbox running: %w", err)
		}
		sb.Status = string(StatusRunning)
		sb.LastActiveAt = &now
		sb.StoppedAt = nil
		sb.ErrorMessage = nil
		if err := o.pushAgentRuntimeConfig(ctx, sb, "restart"); err != nil {
			return err
		}
		return nil
	}
	if err := o.StopSandbox(ctx, sb); err != nil {
		return err
	}
	return o.StartAgentSandbox(ctx, sb)
}

func (o *Orchestrator) NeedsURLRefresh(sb *model.Sandbox) bool {
	return o.needsURLRefresh(sb)
}
