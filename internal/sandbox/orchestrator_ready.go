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
