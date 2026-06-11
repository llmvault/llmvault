package sandbox

import (
	"context"
	"fmt"
	"time"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

func (o *Orchestrator) WakeSandbox(ctx context.Context, sb *model.Sandbox) (*model.Sandbox, error) {
	if err := o.ensureSandboxProvider(sb); err != nil {
		return nil, err
	}
	if err := o.provider.StartSandbox(ctx, sb.ExternalID); err != nil {
		return nil, fmt.Errorf("starting sandbox %s: %w", sb.ID, err)
	}

	if err := o.RefreshEmployeeSandboxURL(ctx, sb); err != nil {
		return nil, fmt.Errorf("refreshing runtime URL after wake: %w", err)
	}

	// Only flip the row to 'running' after the runtime is confirmed healthy.
	// Persisting 'running' before the health wait lets a concurrent
	// EnsureSandboxActive observe an in-memory/DB 'running' for a sandbox that
	// never came back, skipping a real wake and routing traffic to a dead URL.
	if err := o.waitForEmployeeRuntimeLive(ctx, sb); err != nil {
		if dbErr := o.db.Model(sb).Updates(map[string]any{
			"status":        "error",
			"error_message": fmt.Sprintf("runtime not healthy after wake: %v", err),
		}).Error; dbErr != nil {
			logging.Capture(ctx, fmt.Errorf("persisting error status after failed wake of sandbox %s: %w", sb.ID, dbErr))
		}
		sb.Status = string(StatusError)
		return nil, fmt.Errorf("runtime not healthy after wake: %w", err)
	}

	now := time.Now()
	if err := o.db.Model(sb).Updates(map[string]any{
		"status":         "running",
		"last_active_at": now,
		"stopped_at":     nil,
		"error_message":  nil,
	}).Error; err != nil {
		return nil, fmt.Errorf("marking sandbox %s running after wake: %w", sb.ID, err)
	}
	sb.Status = "running"
	sb.LastActiveAt = &now
	sb.StoppedAt = nil

	logging.FromContext(ctx).InfoContext(ctx, "sandbox woken", "sandbox_id", sb.ID, "external_id", sb.ExternalID)
	return sb, nil
}
