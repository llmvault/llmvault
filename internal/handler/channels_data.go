package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

func (h *ChannelHandler) resolveDefaultAgentID(ctx context.Context, w http.ResponseWriter, orgID uuid.UUID, raw *string) (uuid.UUID, bool) {
	if raw != nil {
		agentID, err := uuid.Parse(cleanStringPtr(raw))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "default_agent_id must be a uuid"})
			return uuid.Nil, false
		}
		if !h.agentBelongsToOrg(ctx, orgID, agentID) {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "default_agent_id must belong to an active agent in this org"})
			return uuid.Nil, false
		}
		return agentID, true
	}
	var agent model.Agent
	err := h.db.WithContext(ctx).
		Where("org_id = ? AND is_default = ? AND status <> ?", orgID, true, "archived").
		First(&agent).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "default_agent_id is required"})
		return uuid.Nil, false
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load default agent"})
		return uuid.Nil, false
	}
	return agent.ID, true
}

func (h *ChannelHandler) agentBelongsToOrg(ctx context.Context, orgID, agentID uuid.UUID) bool {
	var count int64
	if err := h.db.WithContext(ctx).
		Model(&model.Agent{}).
		Where("id = ? AND org_id = ? AND status <> ?", agentID, orgID, "archived").
		Count(&count).Error; err != nil {
		return false
	}
	return count == 1
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
	var members []model.ChannelMember
	_ = h.db.WithContext(ctx).
		Where("channel_id = ?", channelID).
		Order("created_at ASC").
		Find(&members).Error
	out := make([]channelMemberResponse, len(members))
	for i, member := range members {
		out[i] = channelMemberResponse{
			UserID:    member.UserID.String(),
			Role:      member.Role,
			CreatedAt: member.CreatedAt.Format(http.TimeFormat),
		}
	}
	return out
}
