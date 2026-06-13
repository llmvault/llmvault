package handler

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/model"
)

const defaultChannelName = "general"

func createDefaultGeneralChannelTx(ctx context.Context, tx *gorm.DB, orgID, userID, agentID uuid.UUID) (*model.Channel, error) {
	channel := model.Channel{
		OrgID:          orgID,
		Name:           defaultChannelName,
		Description:    "General workspace channel",
		Kind:           "standard",
		Visibility:     "public",
		DefaultAgentID: agentID,
		IsDefault:      true,
		CreatedBy:      &userID,
	}
	if err := tx.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&channel).Error; err != nil {
		return nil, fmt.Errorf("create #general channel: %w", err)
	}
	if channel.ID == uuid.Nil {
		if err := tx.WithContext(ctx).
			Where("org_id = ? AND name = ?", orgID, defaultChannelName).
			First(&channel).Error; err != nil {
			return nil, fmt.Errorf("load #general channel: %w", err)
		}
	}

	member := model.ChannelMember{
		ChannelID: channel.ID,
		UserID:    userID,
		Role:      "owner",
	}
	if err := tx.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&member).Error; err != nil {
		return nil, fmt.Errorf("create #general channel member: %w", err)
	}
	return &channel, nil
}
