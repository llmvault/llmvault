package handler

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

// canViewChannel mirrors canUseChannel: an org admin, or a member of the channel.
func canViewChannel(ctx context.Context, db *gorm.DB, channel model.Channel, orgRole string, userID *uuid.UUID, apiKey bool) bool {
	return canUseChannel(ctx, db, channel, orgRole, userID, apiKey)
}

func canUseChannel(ctx context.Context, db *gorm.DB, channel model.Channel, orgRole string, userID *uuid.UUID, apiKey bool) bool {
	if apiKey || isOrgManager(orgRole) {
		return true
	}
	if userID == nil || orgRole == "" {
		return false
	}
	if channel.Origin == "external" {
		return true
	}
	if channel.TeamID == nil {
		return true
	}
	return userIsActiveTeamMember(ctx, db, *channel.TeamID, *userID)
}

func userIsActiveTeamMember(ctx context.Context, db *gorm.DB, teamID, userID uuid.UUID) bool {
	var count int64
	_ = db.WithContext(ctx).
		Table("team_members").
		Joins("JOIN teams ON teams.id = team_members.team_id AND teams.archived_at IS NULL").
		Where("team_members.team_id = ? AND team_members.user_id = ?", teamID, userID).
		Count(&count).Error
	return count > 0
}

func visibleTeamSubquery(db *gorm.DB, userID *uuid.UUID) *gorm.DB {
	q := db.Model(&model.TeamMember{}).
		Select("team_members.team_id").
		Joins("JOIN teams ON teams.id = team_members.team_id AND teams.archived_at IS NULL")
	if userID == nil {
		return q.Where("1 = 0")
	}
	return q.Where("team_members.user_id = ?", *userID)
}
