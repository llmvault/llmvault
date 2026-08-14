package sandbox

import (
	"context"
	"fmt"
	"time"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

// RunResourceCheck queries all running sandboxes and collects their resource stats.
func (o *Orchestrator) RunResourceCheck(ctx context.Context) {
	var sandboxes []model.Sandbox
	if err := o.db.WithContext(ctx).
		Where("status = ? AND provider_id <> ?", string(StatusRunning), ProviderDesktop).
		Find(&sandboxes).Error; err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "resource check: failed to query sandboxes", "error", err)
		return
	}

	if len(sandboxes) == 0 {
		return
	}

	for i := range sandboxes {
		sb := &sandboxes[i]
		o.collectSandboxResources(ctx, sb)
	}
}

// collectSandboxResources fetches provider resource stats and updates the sandbox record.
func (o *Orchestrator) collectSandboxResources(ctx context.Context, sb *model.Sandbox) {
	if err := o.ensureSandboxProvider(sb); err != nil {
		logging.Capture(ctx, err)
		return
	}
	stats, err := o.provider.GetResourceUsage(ctx, sb.ExternalID)
	if err != nil {
		logging.Capture(ctx, fmt.Errorf("resource check sandbox %s: %w", sb.ID, err))
		return
	}

	now := time.Now()
	updates := map[string]any{
		"memory_limit_bytes":  stats.MemoryLimitBytes,
		"memory_used_bytes":   stats.MemoryUsedBytes,
		"memory_peak_bytes":   stats.MemoryPeakBytes,
		"cpu_quota":           stats.CPUQuota,
		"cpu_usage_usec":      stats.CPUUsageUsec,
		"cpu_throttled_count": stats.CPUThrottledCount,
		"pid_count":           stats.PIDCount,
		"resource_checked_at": now,
	}

	if err := o.db.Model(sb).Updates(updates).Error; err != nil {
		logging.Capture(ctx, fmt.Errorf("resource check db update sandbox %s: %w", sb.ID, err))
		return
	}
}
