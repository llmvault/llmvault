package sandbox

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

// healthErrorThreshold is the number of consecutive bad provider observations
// required before a running sandbox is persisted as error. Railway maps any
// unknown/transient deployment state (e.g. a momentary CRASHED that auto-
// recovers) to error, so persisting on the first observation permanently bricks
// the row — and P0-17 would then provision a duplicate. Requiring N consecutive
// bad reads lets transient blips heal.
const healthErrorThreshold = 3

func (o *Orchestrator) RunHealthCheck(ctx context.Context) {
	var sandboxes []model.Sandbox
	// Include error rows so a sandbox the provider has since recovered can be
	// re-probed back to running instead of being terminal forever.
	if err := o.db.WithContext(ctx).Where("status IN ?", []string{string(StatusRunning), string(StatusError)}).Find(&sandboxes).Error; err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "health check: failed to query sandboxes", "error", err)
		return
	}

	seen := make(map[uuid.UUID]struct{}, len(sandboxes))
	for i := range sandboxes {
		sb := &sandboxes[i]
		seen[sb.ID] = struct{}{}
		o.checkSandboxHealth(ctx, sb)
	}
	o.pruneHealthFailureCounts(seen)
}

func (o *Orchestrator) checkSandboxHealth(ctx context.Context, sb *model.Sandbox) {
	if err := o.ensureSandboxProvider(sb); err != nil {
		logging.Capture(ctx, err)
		return
	}
	status, err := o.provider.GetStatus(ctx, sb.ExternalID)
	if err != nil {
		// Surface GetStatus failures: a silently-swallowed error means a deleted
		// or unreachable resource stays 'running' forever. Don't flip status on a
		// transport error — only on observed terminal provider states.
		logging.Capture(ctx, fmt.Errorf("health check: get status for sandbox %s (%s): %w", sb.ID, sb.ExternalID, err))
		return
	}

	providerStatus := string(status)
	if providerStatus == sb.Status {
		o.clearHealthFailureCount(sb.ID)
		return
	}

	// Error is only persisted after N consecutive bad observations so a single
	// transient CRASHED reading does not brick a sandbox that the provider auto-
	// recovers.
	if providerStatus == string(StatusError) {
		count := o.incrHealthFailureCount(sb.ID)
		if count < healthErrorThreshold {
			logging.FromContext(ctx).DebugContext(ctx, "health check: transient bad status, deferring error",
				"sandbox_id", sb.ID, "observed", providerStatus, "consecutive", count, "threshold", healthErrorThreshold)
			return
		}
		logging.FromContext(ctx).WarnContext(ctx, "health check: persisting error after consecutive bad observations",
			"sandbox_id", sb.ID, "consecutive", count)
	} else {
		o.clearHealthFailureCount(sb.ID)
	}

	logging.FromContext(ctx).DebugContext(ctx, "health check: status changed", "sandbox_id", sb.ID, "old", sb.Status, "new", providerStatus)
	o.db.WithContext(ctx).Model(sb).Update("status", providerStatus)
	sb.Status = providerStatus
}

func (o *Orchestrator) incrHealthFailureCount(id uuid.UUID) int {
	o.healthFailureMu.Lock()
	defer o.healthFailureMu.Unlock()
	if o.healthFailureCounts == nil {
		o.healthFailureCounts = make(map[uuid.UUID]int)
	}
	o.healthFailureCounts[id]++
	return o.healthFailureCounts[id]
}

func (o *Orchestrator) clearHealthFailureCount(id uuid.UUID) {
	o.healthFailureMu.Lock()
	defer o.healthFailureMu.Unlock()
	delete(o.healthFailureCounts, id)
}

// pruneHealthFailureCounts drops counters for sandboxes no longer in the sweep
// so the map cannot grow unbounded across the process lifetime.
func (o *Orchestrator) pruneHealthFailureCounts(seen map[uuid.UUID]struct{}) {
	o.healthFailureMu.Lock()
	defer o.healthFailureMu.Unlock()
	for id := range o.healthFailureCounts {
		if _, ok := seen[id]; !ok {
			delete(o.healthFailureCounts, id)
		}
	}
}
