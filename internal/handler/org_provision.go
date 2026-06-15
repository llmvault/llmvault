package handler

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/tasks"
)

func enqueueOrgHivyAgentProvision(ctx context.Context, enq enqueue.TaskEnqueuer, orgID uuid.UUID, stage string) {
	if enq == nil {
		logging.FromContext(ctx).WarnContext(ctx, "Hivy agent sandbox provision skipped; queue not configured",
			"org_id", orgID, "stage", stage)
		return
	}
	if err := tasks.EnqueueOrgHivyAgentProvision(ctx, enq, orgID); err != nil {
		err = fmt.Errorf("enqueue Hivy agent sandbox provision: %w", err)
		logging.FromContext(ctx).ErrorContext(ctx, "failed to enqueue Hivy agent sandbox provision",
			"org_id", orgID, "stage", stage, "error", err)
		logging.CaptureWithFields(ctx, err, map[string]any{
			"org_id": orgID.String(),
			"stage":  stage,
		})
	}
}
