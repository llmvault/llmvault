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

// provisionTeamDefaults makes a freshly created team self-sufficient: it clones
// the "hivy" catalog agent into the team as the team's undeletable default Hivy
// (IsDefault=true, team_id set, Instructions nil so it auto-syncs with the
// catalog) and creates the team's #general channel (team-scoped, public,
// default agent = that Hivy, with createdByUserID as an owner member).
//
// It is called for every new team — from TeamHandler.Create and from the org
// bootstrap that forces a first team at signup — so no team is ever left
// without a Hivy or a channel.
func provisionTeamDefaults(ctx context.Context, tx *gorm.DB, orgID, teamID, createdByUserID uuid.UUID) (*model.Agent, *model.Channel, error) {
	agent, err := createHivyAgentWithDefaultsTx(ctx, tx, orgID, teamID)
	if err != nil {
		return nil, nil, fmt.Errorf("provision team Hivy agent: %w", err)
	}
	channel, err := createDefaultGeneralChannelTx(ctx, tx, orgID, teamID, createdByUserID, agent.ID)
	if err != nil {
		return nil, nil, err
	}
	return agent, channel, nil
}

func createDefaultGeneralChannelTx(ctx context.Context, tx *gorm.DB, orgID, teamID, userID, agentID uuid.UUID) (*model.Channel, error) {
	channel := model.Channel{
		OrgID:          orgID,
		TeamID:         teamID,
		Name:           defaultChannelName,
		Description:    "General workspace channel",
		Kind:           "standard",
		Visibility:     "public",
		DefaultAgentID: agentID,
		IsDefault:      true,
	}
	// userID is uuid.Nil when the team is created by an API key (no user actor);
	// leave created_by/member unset in that case rather than writing a bogus FK.
	if userID != uuid.Nil {
		createdBy := userID
		channel.CreatedBy = &createdBy
	}
	if err := tx.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&channel).Error; err != nil {
		return nil, fmt.Errorf("create #general channel: %w", err)
	}
	if channel.ID == uuid.Nil {
		if err := tx.WithContext(ctx).
			Where("org_id = ? AND team_id = ? AND name = ?", orgID, teamID, defaultChannelName).
			First(&channel).Error; err != nil {
			return nil, fmt.Errorf("load #general channel: %w", err)
		}
	}

	if userID != uuid.Nil {
		member := model.ChannelMember{
			ChannelID: channel.ID,
			UserID:    userID,
			Role:      "owner",
		}
		if err := tx.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&member).Error; err != nil {
			return nil, fmt.Errorf("create #general channel member: %w", err)
		}
	}
	// No assignment row: under the team-primary model the default agent is usable
	// in the channel by virtue of team ownership (agent.team_id == channel.team_id).
	return &channel, nil
}
