package sandbox

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

// healthErrorThreshold is the consecutive bad observations required before a
// running sandbox is persisted as error. Railway maps transient states (a momentary
// CRASHED that auto-recovers) to error, so persisting on the first read would brick
// the row and provision a duplicate.
const healthErrorThreshold = 3

func (o *Orchestrator) RunHealthCheck(ctx context.Context) {
	var sandboxes []model.Sandbox
	// Include error rows so a since-recovered sandbox can be re-probed to running.
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
		// Surface GetStatus failures (a swallowed error leaves a dead resource
		// 'running' forever), but only flip status on observed terminal states.
		logging.Capture(ctx, fmt.Errorf("health check: get status for sandbox %s (%s): %w", sb.ID, sb.ExternalID, err))
		return
	}

	providerStatus := string(status)
	if providerStatus == sb.Status {
		o.clearHealthFailureCount(sb.ID)
		return
	}

	// Persist error only after N consecutive bad observations, so a transient
	// CRASHED that the provider auto-recovers does not brick the sandbox.
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
