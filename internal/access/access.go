// Package access resolves the human actor behind an agent turn and answers
// authorization questions (org role, channel access) for the agent-facing MCP
// tools.
//
// The agent proxy token carries only org + agent identity, so tools cannot
// authorize on the requesting user by themselves. The Rust runtime injects a
// hidden, server-controlled `_hivy_actor_user_id` into every "hivy" tool call
// (the same mechanism as `_hivy_session_id`); the model can neither see nor
// forge it. Tools pass that raw value to Resolve and then gate on the returned
// Actor. This package is the single source of truth for those predicates so the
// agent path and the HTTP API cannot drift: the handler helpers
// (handler.canUseChannel, handler.canManageTeamResource) delegate here rather
// than re-implementing the rule.
package access

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

// Actor is the resolved human on whose behalf an agent turn is running.
type Actor struct {
	UserID  uuid.UUID
	OrgID   uuid.UUID
	OrgRole string // "owner" | "admin" | "member"
}

// Resolve turns the runtime-injected `_hivy_actor_user_id` into an Actor.
//
// It returns (nil, nil) when raw is empty: automated trigger, cron, and system
// runs have no human actor, and callers fall back to their existing
// agent-scoped behavior for those. It returns an error only when a non-empty id
// is malformed or does not belong to a member of orgID — a present-but-invalid
// actor must never be treated as "no actor".
func Resolve(ctx context.Context, db *gorm.DB, orgID uuid.UUID, raw string) (*Actor, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	userID, err := uuid.Parse(raw)
	if err != nil || userID == uuid.Nil {
		return nil, fmt.Errorf("requesting user id is not a valid UUID")
	}
	var membership model.OrgMembership
	// A deactivated member never resolves to a usable Actor (deactivated_at IS NULL).
	err = db.WithContext(ctx).
		Where("org_id = ? AND user_id = ? AND deactivated_at IS NULL", orgID, userID).
		First(&membership).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("the requesting user is not a member of this organization")
	}
	if err != nil {
		return nil, fmt.Errorf("resolve requesting user: %w", err)
	}
	return &Actor{UserID: userID, OrgID: orgID, OrgRole: membership.Role}, nil
}

// IsOrgManager reports whether the actor is an org owner or admin. Nil-safe: a
// nil actor (no human context) is not a manager.
func (a *Actor) IsOrgManager() bool {
	if a == nil {
		return false
	}
	return a.OrgRole == "owner" || a.OrgRole == "admin"
}

// IsOrgOwner reports whether the actor is specifically an org owner (not admin).
// Nil-safe: a nil actor (no human context) is not an owner. This is the
// owner-only tier that IsOrgManager (owner OR admin) deliberately does not
// distinguish.
func (a *Actor) IsOrgOwner() bool {
	if a == nil {
		return false
	}
	return a.OrgRole == "owner"
}

// IsTeamMember reports whether the actor is an active member of the given team
// (an unarchived team with a matching team_members row). Nil-safe: a nil actor
// is not a member of any team.
func (a *Actor) IsTeamMember(ctx context.Context, db *gorm.DB, teamID uuid.UUID) (bool, error) {
	if a == nil {
		return false, nil
	}
	var count int64
	err := db.WithContext(ctx).
		Table("team_members").
		Joins("JOIN teams ON teams.id = team_members.team_id AND teams.archived_at IS NULL").
		Where("team_members.team_id = ? AND team_members.user_id = ? AND team_members.deactivated_at IS NULL", teamID, a.UserID).
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("check team membership: %w", err)
	}
	return count > 0, nil
}

// CanManageTeamResource reports whether the actor may manage a resource owned by
// the given team: org managers (owner/admin) always may, otherwise the actor
// must be an active member of that team. Nil-safe.
func (a *Actor) CanManageTeamResource(ctx context.Context, db *gorm.DB, teamID uuid.UUID) (bool, error) {
	if a == nil {
		return false, nil
	}
	if a.IsOrgManager() {
		return true, nil
	}
	return a.IsTeamMember(ctx, db, teamID)
}

// CanUseChannelID reports whether the actor may use (post/run in) the given
// channel. Mirrors handler.canUseChannel: org managers and API-key paths aside,
// external and non-team channels are open to any org member, while a
// team-scoped channel requires active membership of that team. A nil actor
// (no human context) cannot use any channel; callers decide whether to enforce.
func (a *Actor) CanUseChannelID(ctx context.Context, db *gorm.DB, channelID uuid.UUID) (bool, error) {
	if a == nil {
		return false, nil
	}
	if a.IsOrgManager() {
		return true, nil
	}
	var channel model.Channel
	err := db.WithContext(ctx).
		Where("id = ? AND org_id = ?", channelID, a.OrgID).
		First(&channel).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("load channel: %w", err)
	}
	return a.canUseChannel(ctx, db, channel), nil
}

// CanUseChannel is CanUseChannelID for an already-loaded channel, letting
// list-style callers avoid a per-row reload.
func (a *Actor) CanUseChannel(ctx context.Context, db *gorm.DB, channel model.Channel) bool {
	if a == nil {
		return false
	}
	if a.IsOrgManager() {
		return true
	}
	return a.canUseChannel(ctx, db, channel)
}

func (a *Actor) canUseChannel(ctx context.Context, db *gorm.DB, channel model.Channel) bool {
	if channel.Visibility == "private" {
		return userIsChannelMember(ctx, db, channel.ID, a.UserID)
	}
	if channel.TeamID == nil {
		return true
	}
	return userIsActiveTeamMember(ctx, db, *channel.TeamID, a.UserID)
}

// userIsChannelMember reports whether userID has an explicit channel_members row
// for channelID. Gates private-channel access on the agent path (mirror of
// handler.userIsChannelMember).
func userIsChannelMember(ctx context.Context, db *gorm.DB, channelID, userID uuid.UUID) bool {
	var count int64
	_ = db.WithContext(ctx).
		Table("channel_members").
		Where("channel_id = ? AND user_id = ? AND deactivated_at IS NULL", channelID, userID).
		Count(&count).Error
	return count > 0
}

func userIsActiveTeamMember(ctx context.Context, db *gorm.DB, teamID, userID uuid.UUID) bool {
	var count int64
	_ = db.WithContext(ctx).
		Table("team_members").
		Joins("JOIN teams ON teams.id = team_members.team_id AND teams.archived_at IS NULL").
		Where("team_members.team_id = ? AND team_members.user_id = ? AND team_members.deactivated_at IS NULL", teamID, userID).
		Count(&count).Error
	return count > 0
}
