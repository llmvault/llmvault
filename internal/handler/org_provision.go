package handler

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/logging"
)

var orgAgentProvisionRetryDelays = []time.Duration{
	0,
	500 * time.Millisecond,
	2 * time.Second,
}

func provisionOrgHivyAgent(ctx context.Context, syncer OrgAgentSyncer, orgID uuid.UUID) error {
	if syncer == nil {
		return fmt.Errorf("agent sandbox sync not configured")
	}
	var lastErr error
	for attempt, delay := range orgAgentProvisionRetryDelays {
		if delay > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
		if err := syncer.SyncOrgHivyAgent(ctx, orgID); err != nil {
			lastErr = err
			logging.FromContext(ctx).WarnContext(ctx, "Hivy agent sandbox provision attempt failed",
				"org_id", orgID, "attempt", attempt+1, "error", err)
			continue
		}
		return nil
	}
	err := fmt.Errorf("provision Hivy agent sandbox: %w", lastErr)
	logging.CaptureWithFields(ctx, err, map[string]any{
		"org_id":   orgID.String(),
		"attempts": len(orgAgentProvisionRetryDelays),
	})
	return err
}
