package handler

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/model"
)

func enablePluginForAgent(ctx context.Context, tx *gorm.DB, orgID, agentID, pluginID uuid.UUID) error {
	install := model.AgentPluginInstall{OrgID: orgID, AgentID: agentID, PluginID: pluginID}
	if err := tx.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&install).Error; err != nil {
		return fmt.Errorf("enable plugin for agent: %w", err)
	}
	var skills []model.Skill
	if err := tx.WithContext(ctx).
		Where("plugin_id = ? AND status = ?", pluginID, model.SkillStatusPublished).
		Find(&skills).Error; err != nil {
		return fmt.Errorf("load plugin skills: %w", err)
	}
	for _, skill := range skills {
		link := model.AgentSkill{AgentID: agentID, SkillID: skill.ID}
		if err := tx.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&link).Error; err != nil {
			return fmt.Errorf("attach plugin skill: %w", err)
		}
	}
	return refreshPluginSkillInstallCounts(ctx, tx, pluginID)
}

func disablePluginForOrg(ctx context.Context, tx *gorm.DB, orgID, pluginID uuid.UUID) error {
	var installs []model.AgentPluginInstall
	if err := tx.WithContext(ctx).Where("org_id = ? AND plugin_id = ?", orgID, pluginID).Find(&installs).Error; err != nil {
		return err
	}
	for _, install := range installs {
		if err := disablePluginForAgent(ctx, tx, orgID, install.AgentID, pluginID); err != nil {
			return err
		}
	}
	return nil
}

func disablePluginForAgent(ctx context.Context, tx *gorm.DB, orgID, agentID, pluginID uuid.UUID) error {
	if err := tx.WithContext(ctx).
		Where("org_id = ? AND agent_id = ? AND plugin_id = ?", orgID, agentID, pluginID).
		Delete(&model.AgentPluginInstall{}).Error; err != nil {
		return err
	}
	var skillIDs []uuid.UUID
	if err := tx.WithContext(ctx).Model(&model.Skill{}).
		Where("plugin_id = ?", pluginID).
		Pluck("id", &skillIDs).Error; err != nil {
		return err
	}
	if len(skillIDs) > 0 {
		if err := tx.WithContext(ctx).
			Where("agent_id = ? AND skill_id IN ?", agentID, skillIDs).
			Delete(&model.AgentSkill{}).Error; err != nil {
			return err
		}
	}
	return refreshPluginSkillInstallCounts(ctx, tx, pluginID)
}

func refreshPluginSkillInstallCounts(ctx context.Context, tx *gorm.DB, pluginID uuid.UUID) error {
	var skills []model.Skill
	if err := tx.WithContext(ctx).Where("plugin_id = ?", pluginID).Find(&skills).Error; err != nil {
		return err
	}
	for _, skill := range skills {
		if err := tx.WithContext(ctx).Model(&model.Skill{}).
			Where("id = ?", skill.ID).
			UpdateColumn("install_count", gorm.Expr("(SELECT COUNT(*) FROM agent_skills WHERE skill_id = ?)", skill.ID)).Error; err != nil {
			return err
		}
	}
	return nil
}
