package plugins

import (
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

func replacePluginIntegrations(tx *gorm.DB, pluginID uuid.UUID, reqs []ConnectionRequirement) error {
	if err := tx.Where("plugin_id = ?", pluginID).Delete(&model.PluginIntegration{}).Error; err != nil {
		return err
	}
	for _, req := range reqs {
		required := true
		if req.Required != nil {
			required = *req.Required
		}
		kind := strings.TrimSpace(req.Kind)
		if kind == "" {
			kind = model.PluginIntegrationKindIntegration
		}
		row := model.PluginIntegration{
			PluginID: pluginID,
			Provider: strings.TrimSpace(req.Provider),
			Kind:     kind,
			Required: required,
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
	}
	return nil
}
