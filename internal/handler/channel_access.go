package handler

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

func canViewChannel(ctx context.Context, db *gorm.DB, channel model.Channel, orgRole string, userID *uuid.UUID, apiKey bool) bool {
	if apiKey || isOrgManager(orgRole) {
		return true
	}
	if userID == nil || orgRole == "" {
		return false
	}
	if channel.TeamID == nil {
		return true
	}
	if userIsActiveTeamMember(ctx, db, *channel.TeamID, *userID) {
		return true
	}
	return userHasSessionInChannel(ctx, db, channel.ID, *userID)
}

func canUseChannel(ctx context.Context, db *gorm.DB, channel model.Channel, orgRole string, userID *uuid.UUID, apiKey bool) bool {
	if apiKey || isOrgManager(orgRole) {
		return true
	}
	if userID == nil || orgRole == "" {
		return false
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

func userHasSessionInChannel(ctx context.Context, db *gorm.DB, channelID, userID uuid.UUID) bool {
	var count int64
	_ = db.WithContext(ctx).
		Model(&model.Session{}).
		Where("channel_id = ? AND (created_by = ? OR id IN (?))", channelID, userID, db.Model(&model.SessionParticipant{}).Select("session_id").Where("user_id = ?", userID)).
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

func participantChannelSubquery(db *gorm.DB, userID *uuid.UUID) *gorm.DB {
	q := db.Model(&model.Session{}).Select("DISTINCT channel_id")
	if userID == nil {
		return q.Where("1 = 0")
	}
	return q.Where("created_by = ? OR id IN (?)", *userID, db.Model(&model.SessionParticipant{}).Select("session_id").Where("user_id = ?", *userID))
}
