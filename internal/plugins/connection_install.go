package plugins

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/model"
)

// InstallForConnection installs every active, org-visible plugin that declares
// the provider as an integration requirement. Provider keys are distinct, so
// GitHub and GitHub code reviews install their own matching plugins.
func InstallForConnection(ctx context.Context, tx *gorm.DB, orgID, userID uuid.UUID, provider string) ([]uuid.UUID, error) {
	var pluginIDs []uuid.UUID
	if err := tx.WithContext(ctx).
		Model(&model.Plugin{}).
		Select("plugins.id").
		Joins("JOIN plugin_integrations ON plugin_integrations.plugin_id = plugins.id").
		Where("plugin_integrations.provider = ? AND plugin_integrations.kind = ?", provider, model.PluginIntegrationKindIntegration).
		Where("plugins.status = ? AND (plugins.org_id IS NULL OR plugins.org_id = ?)", model.PluginStatusActive, orgID).
		Order("plugins.slug ASC").
		Pluck("plugins.id", &pluginIDs).Error; err != nil {
		return nil, fmt.Errorf("load plugins for connection: %w", err)
	}

	for _, pluginID := range pluginIDs {
		createdBy := userID
		install := model.OrgPluginInstall{
			ID:              uuid.New(),
			OrgID:           orgID,
			PluginID:        pluginID,
			CreatedByUserID: &createdBy,
		}
		if err := tx.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&install).Error; err != nil {
			return nil, fmt.Errorf("install connection plugin: %w", err)
		}
		if err := RefreshPluginSkillInstallCounts(ctx, tx, pluginID); err != nil {
			return nil, err
		}
	}
	return pluginIDs, nil
}
