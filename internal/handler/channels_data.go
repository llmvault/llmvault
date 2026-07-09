package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

// isTeamLastChannel reports whether the given channel is the only non-archived
// channel of its team. Team-less/external channels (team_id NULL) are exempt
// (never the "last channel" of a team) so this returns false for them.
func (h *ChannelHandler) isTeamLastChannel(ctx context.Context, channel model.Channel) (bool, error) {
	if channel.TeamID == nil {
		return false, nil
	}
	var count int64
	if err := h.db.WithContext(ctx).
		Model(&model.Channel{}).
		Where("org_id = ? AND team_id = ? AND archived_at IS NULL", channel.OrgID, *channel.TeamID).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count <= 1, nil
}

// resolveDefaultAgentID resolves the channel's default agent — either the caller
// supplied one (raw) or the org's default agent — and enforces that a
// team-scoped channel's default agent belongs to the same team. Under the
// team-primary model the default agent must be usable in the channel, i.e. it
// must belong to the channel's team (see channelagents.ActsInChannel); this gate
// is what keeps a cross-team agent from being made a channel default. teamID is
// the channel's (would-be) team; the cross-team rule applies only when both the
// channel and the agent are team-scoped (both non-null).
func (h *ChannelHandler) resolveDefaultAgentID(ctx context.Context, w http.ResponseWriter, orgID uuid.UUID, teamID *uuid.UUID, raw *string) (uuid.UUID, bool) {
	var agent model.Agent
	if raw != nil {
		agentID, err := uuid.Parse(cleanStringPtr(raw))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "default_agent_id must be a uuid"})
			return uuid.Nil, false
		}
		err = h.db.WithContext(ctx).
			Where("id = ? AND org_id = ? AND status <> ?", agentID, orgID, "archived").
			First(&agent).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "default_agent_id must belong to an active agent in this org"})
			return uuid.Nil, false
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load default agent"})
			return uuid.Nil, false
		}
	} else {
		err := h.db.WithContext(ctx).
			Where("org_id = ? AND is_default = ? AND status <> ? AND parent_agent_id IS NULL", orgID, true, "archived").
			First(&agent).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "default_agent_id is required"})
			return uuid.Nil, false
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load default agent"})
			return uuid.Nil, false
		}
	}
	if teamID != nil && *teamID != agent.TeamID {
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: "default agent belongs to a different team than this channel"})
		return uuid.Nil, false
	}
	return agent.ID, true
}

// sameTeamBinding reports whether two optional team ids are the same binding
// (both nil, or both the same non-nil id).
func sameTeamBinding(a, b *uuid.UUID) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// authorizeChannelTeamMove gates a PATCH that changes a channel's team binding.
// The Update handler already verified the caller manages the channel's CURRENT
// team; this additionally enforces, only when the binding actually changes:
//   - the team-anchored default channel (#general) can never be re-teamed;
//   - the move must not leave the source team with zero channels (last-channel floor);
//   - clearing the team to NULL (which exposes the channel to every org member) is
//     manager-only;
//   - moving into a team requires the caller to manage the DESTINATION team, so a
//     member cannot hand a channel (and its sessions/env-vars/knowledge) to a team
//     they don't belong to.
//
// Writes the appropriate 4xx/5xx and returns false when the move must be blocked.
func (h *ChannelHandler) authorizeChannelTeamMove(w http.ResponseWriter, r *http.Request, channel model.Channel, newTeamID *uuid.UUID) bool {
	if sameTeamBinding(channel.TeamID, newTeamID) {
		return true
	}
	ctx := r.Context()
	userID, _ := currentRequestUserID(ctx)
	role, err := h.currentUserOrgRole(ctx, channel.OrgID, userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load org membership"})
		return false
	}
	apiKey := isAPIKeyRequest(ctx)

	if channel.IsDefault {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "the default channel cannot be moved to another team"})
		return false
	}
	if last, err := h.isTeamLastChannel(ctx, channel); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to check team channels"})
		return false
	} else if last {
		writeJSON(w, http.StatusConflict, errorResponse{Error: "a team must keep at least one channel"})
		return false
	}
	if newTeamID == nil {
		if !apiKey && !isOrgManager(role) {
			writeJSON(w, http.StatusForbidden, errorResponse{Error: "only an org manager can remove a channel from its team"})
			return false
		}
		return true
	}
	// canManageChannel probed against the destination team is exactly "the caller
	// can manage that team" (API key / org manager / active team member).
	if !canManageChannel(ctx, h.db, model.Channel{OrgID: channel.OrgID, TeamID: newTeamID}, userID, role, apiKey) {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "you must be able to manage the destination team to move this channel into it"})
		return false
	}
	return true
}

func (h *ChannelHandler) resolveTeamID(ctx context.Context, w http.ResponseWriter, orgID uuid.UUID, raw *string) (*uuid.UUID, bool) {
	if raw == nil {
		return nil, true
	}
	value := cleanStringPtr(raw)
	if value == "" {
		return nil, true
	}
	teamID, err := uuid.Parse(value)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "team_id must be a uuid"})
		return nil, false
	}
	var count int64
	if err := h.db.WithContext(ctx).
		Model(&model.Team{}).
		Where("id = ? AND org_id = ? AND archived_at IS NULL", teamID, orgID).
		Count(&count).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load team"})
		return nil, false
	}
	if count != 1 {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "team_id must belong to an active team in this org"})
		return nil, false
	}
	return &teamID, true
}

func (h *ChannelHandler) memberCount(ctx context.Context, channelID uuid.UUID) int64 {
	var count int64
	_ = h.db.WithContext(ctx).
		Model(&model.ChannelMember{}).
		Where("channel_id = ? AND deactivated_at IS NULL", channelID).
		Count(&count).Error
	return count
}

func (h *ChannelHandler) memberCounts(ctx context.Context, channelIDs []uuid.UUID) map[uuid.UUID]int64 {
	out := make(map[uuid.UUID]int64, len(channelIDs))
	if len(channelIDs) == 0 {
		return out
	}
	type row struct {
		ChannelID uuid.UUID
		Count     int64
	}
	var rows []row
	_ = h.db.WithContext(ctx).
		Model(&model.ChannelMember{}).
		Select("channel_id, count(*) AS count").
		Where("channel_id IN ? AND deactivated_at IS NULL", channelIDs).
		Group("channel_id").
		Scan(&rows).Error
	for _, row := range rows {
		out[row.ChannelID] = row.Count
	}
	return out
}

func (h *ChannelHandler) channelRoles(ctx context.Context, channelIDs []uuid.UUID, userID *uuid.UUID) map[uuid.UUID]string {
	out := make(map[uuid.UUID]string, len(channelIDs))
	if len(channelIDs) == 0 || userID == nil {
		return out
	}
	var members []model.ChannelMember
	_ = h.db.WithContext(ctx).
		Where("channel_id IN ? AND user_id = ? AND deactivated_at IS NULL", channelIDs, *userID).
		Find(&members).Error
	for _, member := range members {
		out[member.ChannelID] = member.Role
	}
	return out
}

func (h *ChannelHandler) channelMembers(ctx context.Context, channelID uuid.UUID) []channelMemberResponse {
	var rows []struct {
		model.ChannelMember
		Name  string `gorm:"column:name"`
		Email string `gorm:"column:email"`
	}
	_ = h.db.WithContext(ctx).
		Table("channel_members").
		Select("channel_members.*, users.name AS name, users.email AS email").
		Joins("LEFT JOIN users ON users.id = channel_members.user_id").
		Where("channel_members.channel_id = ? AND channel_members.deactivated_at IS NULL", channelID).
		Order("channel_members.created_at ASC").
		Scan(&rows).Error
	out := make([]channelMemberResponse, len(rows))
	for i, member := range rows {
		out[i] = channelMemberResponse{
			UserID:    member.UserID.String(),
			Name:      member.Name,
			Email:     member.Email,
			Role:      member.Role,
			CreatedAt: member.CreatedAt.Format(http.TimeFormat),
		}
	}
	return out
}
