package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

// resolveDefaultAgentID resolves the channel's default agent — either the caller
// supplied one (raw) or the org's default agent — and enforces that a
// team-scoped channel's default agent belongs to the same team. The default
// agent is auto-assigned to the channel (channel_agents) by virtue of being the
// default, so it must clear the same same-team gate AssignChannelAgent applies;
// otherwise picking a cross-team default agent would bypass that check. teamID
// is the channel's (would-be) team; the cross-team rule applies only when both
// the channel and the agent are team-scoped (both non-null).
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
	if teamID != nil && agent.TeamID != nil && *teamID != *agent.TeamID {
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: "default agent belongs to a different team than this channel"})
		return uuid.Nil, false
	}
	return agent.ID, true
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
		Where("channel_id = ?", channelID).
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
		Where("channel_id IN ?", channelIDs).
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
		Where("channel_id IN ? AND user_id = ?", channelIDs, *userID).
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
		Where("channel_members.channel_id = ?", channelID).
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
