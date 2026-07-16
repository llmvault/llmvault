package bootstrap

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/agentcatalog"
	"github.com/usehivy/hivy/internal/billing/plancatalog"
	"github.com/usehivy/hivy/internal/logging"
)

func syncGlobalAgents(ctx context.Context, database *gorm.DB) error {
	result, err := agentcatalog.SyncLocal(ctx, database, "global/agents")
	if err != nil {
		return fmt.Errorf("syncing global agents: %w", err)
	}
	logging.FromContext(ctx).InfoContext(ctx, "global agents synced",
		"created", result.Created,
		"updated", result.Updated,
		"archived", result.Archived,
	)
	return nil
}

func seedGlobalPlans(ctx context.Context, database *gorm.DB) error {
	result, err := plancatalog.SyncDB(ctx, database, "global/plans/catalog.json")
	if err != nil {
		return fmt.Errorf("seeding global plans: %w", err)
	}
	logging.FromContext(ctx).InfoContext(ctx, "global plans seeded",
		"created", result.Created,
		"updated", result.Updated,
		"unchanged", result.Unchanged,
	)
	return nil
}
