package sandbox

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/usehivy/hivy/internal/model"
)

func (o *Orchestrator) EnsureSandboxActive(ctx context.Context, sb *model.Sandbox) (*model.Sandbox, error) {
	if err := o.ensureSandboxProvider(sb); err != nil {
		return nil, err
	}
	switch sb.Status {
	case string(StatusRunning):
		return sb, nil

	case string(StatusStopped):
		return o.WakeSandbox(ctx, sb)

	case string(StatusArchived), string(StatusArchiving):
		return o.UnarchiveSandbox(ctx, sb)

	case string(StatusCreating), string(StatusStarting):
		// A 'creating'/'starting' row may not have runtime_url yet (concurrent warm
		// claim in flight). Probing an empty URL hammers localhost/healthz for the
		// full timeout, so poll for a populated URL first and fail fast otherwise.
		if err := o.waitForPopulatedRuntimeURL(ctx, sb); err != nil {
			return nil, err
		}
		if err := o.waitForEmployeeRuntimeLive(ctx, sb); err != nil {
			return nil, fmt.Errorf("waiting for in-flight sandbox: %w", err)
		}
		now := time.Now()
		// Guard the flip so a concurrent transition (rollback to 'error', delete) is
		// not overwritten with 'running'.
		res := o.db.WithContext(ctx).Model(&model.Sandbox{}).
			Where("id = ? AND status IN ?", sb.ID, []string{string(StatusCreating), string(StatusStarting)}).
			Updates(map[string]any{
				"status":         "running",
				"last_active_at": now,
			})
		if res.Error != nil {
			return nil, fmt.Errorf("activating in-flight sandbox %s: %w", sb.ID, res.Error)
		}
		if res.RowsAffected == 0 {
			// Something else moved the row; reload and re-resolve, not assert stale 'running'.
			if err := o.db.WithContext(ctx).First(sb, "id = ?", sb.ID).Error; err != nil {
				return nil, fmt.Errorf("reloading in-flight sandbox %s: %w", sb.ID, err)
			}
			if sb.Status == string(StatusRunning) {
				return sb, nil
			}
			return o.EnsureSandboxActive(ctx, sb)
		}
		sb.Status = "running"
		sb.LastActiveAt = &now
		return sb, nil

	case string(StatusError):
		return nil, fmt.Errorf("sandbox %s is in error state", sb.ID)

	case string(StatusUpgrading):
		// Mid-upgrade and intentionally non-selectable; surface a clear error
		// rather than probing or flipping it.
		return nil, fmt.Errorf("sandbox %s is mid-upgrade", sb.ID)

	default:
		status, err := o.provider.GetStatus(ctx, sb.ExternalID)
		if err != nil {
			return nil, fmt.Errorf("getting provider status for sandbox %s: %w", sb.ID, err)
		}
		sb.Status = string(status)
		if err := o.db.Model(sb).Update("status", sb.Status).Error; err != nil {
			return nil, fmt.Errorf("reconciling provider status for sandbox %s: %w", sb.ID, err)
		}
		if sb.Status == string(StatusRunning) {
			return sb, nil
		}
		return o.EnsureSandboxActive(ctx, sb)
	}
}

const inFlightRuntimeURLInterval = 1 * time.Second

// waitForPopulatedRuntimeURL polls until runtime_url is populated (written by a
// concurrent create/warm-claim), failing fast if it never appears.
func (o *Orchestrator) waitForPopulatedRuntimeURL(ctx context.Context, sb *model.Sandbox) error {
	if strings.TrimSpace(sb.RuntimeURL) != "" {
		return nil
	}
	deadline := time.Now().Add(employeeHealthTimeout)
	for time.Now().Before(deadline) {
		var current model.Sandbox
		if err := o.db.WithContext(ctx).Select("status", "runtime_url").First(&current, "id = ?", sb.ID).Error; err != nil {
			return fmt.Errorf("polling in-flight sandbox %s runtime url: %w", sb.ID, err)
		}
		if current.Status == string(StatusError) {
			return fmt.Errorf("sandbox %s entered error state while waiting for runtime url", sb.ID)
		}
		if strings.TrimSpace(current.RuntimeURL) != "" {
			sb.RuntimeURL = current.RuntimeURL
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(inFlightRuntimeURLInterval):
		}
	}
	return fmt.Errorf("sandbox %s runtime url not populated within %s", sb.ID, employeeHealthTimeout)
}
