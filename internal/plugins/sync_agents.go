package plugins

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

// archivePlugin marks a plugin as archived. Agents inherit skills dynamically
// from their installed plugins, so there is nothing else to detach here.
func archivePlugin(ctx context.Context, tx *gorm.DB, pluginID uuid.UUID) error {
	if err := tx.WithContext(ctx).Model(&model.Plugin{}).
		Where("id = ?", pluginID).
		Update("status", model.PluginStatusArchived).Error; err != nil {
		return fmt.Errorf("archive plugin: %w", err)
	}
	return nil
}
