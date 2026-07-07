package handler

import (
	"context"
	"errors"

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

// orgRoleForUser resolves a user's org-membership role, returning "" when the
// user has no membership (or is nil). It is the free-function analogue of the
// per-handler currentUserOrgRole/orgRole methods, for handlers that lack one.
func orgRoleForUser(ctx context.Context, db *gorm.DB, orgID uuid.UUID, userID *uuid.UUID) (string, error) {
	if userID == nil {
		return "", nil
	}
	var membership model.OrgMembership
	err := db.WithContext(ctx).
		Where("org_id = ? AND user_id = ?", orgID, *userID).
		First(&membership).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	return membership.Role, err
}

// actorSeesOrgWide reports whether the current request is entitled to org-wide
// visibility — an API-key caller, or an org owner/admin. It also returns the
// resolved acting user id (nil for API-key callers), used to build the
// team-scoped visibility subqueries applied to non-manager members.
func actorSeesOrgWide(ctx context.Context, db *gorm.DB, orgID uuid.UUID) (bool, *uuid.UUID, error) {
	if isAPIKeyRequest(ctx) {
		return true, nil, nil
	}
	userID, _ := currentRequestUserID(ctx)
	role, err := orgRoleForUser(ctx, db, orgID, userID)
	if err != nil {
		return false, userID, err
	}
	return isOrgManager(role), userID, nil
}

// visibleSessionIDSubquery yields the ids of sessions whose channel the given
// user may view, per the team-based channel-visibility predicate (mirrors
// canUseChannel). Used to scope session-derived rows (e.g. canvas artifacts) to
// what a non-manager member is allowed to reach.
func visibleSessionIDSubquery(db *gorm.DB, orgID uuid.UUID, userID *uuid.UUID) *gorm.DB {
	return db.Model(&model.Session{}).
		Select("sessions.id").
		Joins("JOIN channels ON channels.id = sessions.channel_id AND channels.archived_at IS NULL").
		Where("sessions.org_id = ?", orgID).
		Where("channels.origin = ? OR channels.team_id IS NULL OR channels.team_id IN (?)",
			"external", visibleTeamSubquery(db, userID))
}

// usableRagSourceIDs returns the ids (as strings for the qdrant source filter)
// of RAG sources granted to at least one channel the given user may use. RAG
// access is deny-by-default and channel-scoped (see internal/rag/mcptools.go):
// a source reachable through no usable channel is invisible to the member.
func usableRagSourceIDs(ctx context.Context, db *gorm.DB, orgID uuid.UUID, userID *uuid.UUID) ([]string, error) {
	var ids []uuid.UUID
	err := db.WithContext(ctx).
		Model(&model.ChannelRagSource{}).
		Distinct("channel_rag_sources.rag_source_id").
		Joins("JOIN channels ON channels.id = channel_rag_sources.channel_id AND channels.archived_at IS NULL").
		Where("channel_rag_sources.org_id = ?", orgID).
		Where("channels.origin = ? OR channels.team_id IS NULL OR channels.team_id IN (?)",
			"external", visibleTeamSubquery(db, userID)).
		Pluck("channel_rag_sources.rag_source_id", &ids).Error
	if err != nil {
		return nil, err
	}
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = id.String()
	}
	return out, nil
}
