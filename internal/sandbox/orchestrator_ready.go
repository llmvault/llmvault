package sandbox

import (
	"context"
	"fmt"

	"github.com/usehivy/hivy/internal/model"
)

func (o *Orchestrator) EnsureSandboxRuntimeReady(ctx context.Context, sb *model.Sandbox) (*model.Sandbox, error) {
	if o == nil {
		return nil, fmt.Errorf("sandbox orchestrator is not configured")
	}
	if sb == nil {
		return nil, fmt.Errorf("sandbox is required")
	}
	if err := o.reconcileRunningSandboxStatus(ctx, sb); err != nil {
		return nil, err
	}
	active, err := o.EnsureSandboxActive(ctx, sb)
	if err != nil {
		return nil, err
	}
	if o.needsURLRefresh(active) {
		if err := o.RefreshAgentSandboxURL(ctx, active); err != nil {
			return nil, fmt.Errorf("refreshing runtime URL: %w", err)
		}
	}
	if err := o.waitForAgentRuntimeLive(ctx, active); err != nil {
		return nil, err
	}
	o.touchLastActive(ctx, active)
	return active, nil
}

func (o *Orchestrator) reconcileRunningSandboxStatus(ctx context.Context, sb *model.Sandbox) error {
	if sb.Status != string(StatusRunning) {
		return nil
	}
	if err := o.ensureSandboxProvider(sb); err != nil {
		return err
	}
	status, err := o.provider.GetStatus(ctx, sb.ExternalID)
	if err != nil {
		return fmt.Errorf("getting provider status for sandbox %s: %w", sb.ID, err)
	}
	if string(status) == sb.Status {
		return nil
	}
	sb.Status = string(status)
	if err := o.db.WithContext(ctx).Model(sb).Update("status", sb.Status).Error; err != nil {
		return fmt.Errorf("reconciling provider status for sandbox %s: %w", sb.ID, err)
	}
	return nil
}
