package bootstrap

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/agentcatalog"
	"github.com/usehivy/hivy/internal/billing/plancatalog"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/plugins"
)

func syncGlobalPlugins(ctx context.Context, database *gorm.DB) error {
	result, err := plugins.SyncLocal(ctx, database, "global/plugins")
	if err != nil {
		return fmt.Errorf("syncing global plugins: %w", err)
	}
	logging.FromContext(ctx).InfoContext(ctx, "global plugins synced",
		"plugins_created", result.PluginsCreated,
		"plugins_updated", result.PluginsUpdated,
		"plugins_archived", result.PluginsArchived,
		"skills_created", result.SkillsCreated,
		"skills_updated", result.SkillsUpdated,
		"skills_archived", result.SkillsArchived,
	)
	return nil
}

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
