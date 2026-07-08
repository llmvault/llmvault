package handler

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/access"
	"github.com/usehivy/hivy/internal/model"
)

// canViewChannel mirrors canUseChannel: an org admin, or a member of the channel.
func canViewChannel(ctx context.Context, db *gorm.DB, channel model.Channel, orgRole string, userID *uuid.UUID, apiKey bool) bool {
	return canUseChannel(ctx, db, channel, orgRole, userID, apiKey)
}

// canUseChannel is the HTTP entry point for the channel-use predicate. It
// delegates to internal/access — the single implementation — so the agent path
// and the HTTP API can never drift. API-key callers keep org-wide access; a
// caller with no resolved membership (nil user or empty role) is denied.
func canUseChannel(ctx context.Context, db *gorm.DB, channel model.Channel, orgRole string, userID *uuid.UUID, apiKey bool) bool {
	if apiKey {
		return true
	}
	if userID == nil || orgRole == "" {
		return false
	}
	actor := &access.Actor{UserID: *userID, OrgID: channel.OrgID, OrgRole: orgRole}
	return actor.CanUseChannel(ctx, db, channel)
}

// memberChannelIDSubquery yields the ids of channels the given user is an
// explicit member of. Used to re-admit private channels to the team-scoped
// visibility predicates (a private channel is excluded from the origin/team
// opening but visible to its members).
func memberChannelIDSubquery(db *gorm.DB, userID *uuid.UUID) *gorm.DB {
	q := db.Model(&model.ChannelMember{}).Select("channel_id").Where("deactivated_at IS NULL")
	if userID == nil {
		return q.Where("1 = 0")
	}
	return q.Where("user_id = ?", *userID)
}

// canManageTeamResource is the HTTP entry point for the team-management
// predicate. It delegates to internal/access — the single implementation — so
// the agent path and the HTTP API can never drift: org managers (owner/admin)
// always may, otherwise the caller must be an active member of that team.
func canManageTeamResource(ctx context.Context, db *gorm.DB, orgID uuid.UUID, userID *uuid.UUID, role string, teamID uuid.UUID) bool {
	if isOrgManager(role) {
		return true
	}
	if userID == nil {
		return false
	}
	actor := &access.Actor{UserID: *userID, OrgID: orgID, OrgRole: role}
	ok, _ := actor.CanManageTeamResource(ctx, db, teamID)
	return ok
}

func visibleTeamSubquery(db *gorm.DB, userID *uuid.UUID) *gorm.DB {
	q := db.Model(&model.TeamMember{}).
		Select("team_members.team_id").
		Joins("JOIN teams ON teams.id = team_members.team_id AND teams.archived_at IS NULL").
		Where("team_members.deactivated_at IS NULL")
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
	// A deactivated member resolves to no role (deactivated_at IS NULL), so every
	// role-gated predicate that flows from here denies them.
	err := db.WithContext(ctx).
		Where("org_id = ? AND user_id = ? AND deactivated_at IS NULL", orgID, *userID).
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
		Where("(channels.visibility <> ? AND (channels.origin = ? OR channels.team_id IS NULL OR channels.team_id IN (?))) OR channels.id IN (?)",
			"private", "external", visibleTeamSubquery(db, userID), memberChannelIDSubquery(db, userID))
}

// usableRagSourceIDs returns the ids (as strings for the qdrant source filter)
// of RAG sources granted to at least one channel the given user may use. RAG
// access is deny-by-default and channel-scoped (see internal/rag/mcptools.go):
// a source reachable through no usable channel is invisible to the member.
func usableRagSourceIDs(ctx context.Context, db *gorm.DB, orgID uuid.UUID, userID *uuid.UUID) ([]string, error) {
	var ids []uuid.UUID
	// RAG grants are team-derived: a source is usable if it is granted to a team
	// the user can see. Channel-scoped grants (channel_rag_sources) were removed.
	err := db.WithContext(ctx).
		Model(&model.TeamRagSource{}).
		Distinct("team_rag_sources.rag_source_id").
		Where("team_rag_sources.org_id = ?", orgID).
		Where("team_rag_sources.team_id IN (?)", visibleTeamSubquery(db, userID)).
		Pluck("team_rag_sources.rag_source_id", &ids).Error
	if err != nil {
		return nil, err
	}
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = id.String()
	}
	return out, nil
}
