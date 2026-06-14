package plugins

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/model"
)

func attachSkillToEnabledAgents(ctx context.Context, tx *gorm.DB, pluginID, skillID uuid.UUID) error {
	var installs []model.AgentPluginInstall
	if err := tx.WithContext(ctx).Where("plugin_id = ?", pluginID).Find(&installs).Error; err != nil {
		return fmt.Errorf("list enabled plugin agents: %w", err)
	}
	for _, install := range installs {
		link := model.AgentSkill{AgentID: install.AgentID, SkillID: skillID}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&link).Error; err != nil {
			return fmt.Errorf("attach plugin skill to agent: %w", err)
		}
	}
	return nil
}

func archivePlugin(ctx context.Context, tx *gorm.DB, pluginID uuid.UUID) error {
	if err := tx.WithContext(ctx).Model(&model.Plugin{}).
		Where("id = ?", pluginID).
		Update("status", model.PluginStatusArchived).Error; err != nil {
		return fmt.Errorf("archive plugin: %w", err)
	}
	var skillIDs []uuid.UUID
	if err := tx.WithContext(ctx).Model(&model.Skill{}).
		Where("plugin_id = ?", pluginID).
		Pluck("id", &skillIDs).Error; err != nil {
		return fmt.Errorf("list archived plugin skills: %w", err)
	}
	if len(skillIDs) > 0 {
		if err := tx.WithContext(ctx).Where("skill_id IN ?", skillIDs).Delete(&model.AgentSkill{}).Error; err != nil {
			return fmt.Errorf("detach archived plugin skills: %w", err)
		}
	}
	return nil
}
