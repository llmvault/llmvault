package sandbox

import (
	"context"
	"fmt"

	"github.com/usehivy/hivy/internal/model"
)

// SleepSandbox puts an idle sandbox to sleep at the control plane and marks it
// 'stopped' locally. Unlike StopSandbox it does the minimum: it only writes the
// status column (leaving runtime_url/stopped_at untouched) so the sandbox wakes
// transparently on the next request and the mark-running path can flip it back.
func (o *Orchestrator) SleepSandbox(ctx context.Context, sb *model.Sandbox) error {
	if err := o.ensureSandboxProvider(sb); err != nil {
		return err
	}

	unlock := o.lifecycle.Lock(sb.ID.String())
	defer unlock()

	if err := o.provider.StopSandbox(ctx, sb.ExternalID); err != nil {
		return fmt.Errorf("sleeping sandbox %s: %w", sb.ID, err)
	}
	if err := o.db.WithContext(ctx).Model(&model.Sandbox{}).
		Where("id = ?", sb.ID).
		Update("status", string(StatusStopped)).Error; err != nil {
		return fmt.Errorf("marking sandbox %s stopped: %w", sb.ID, err)
	}
	return nil
}
