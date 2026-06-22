package handler

import (
	"context"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

func (h *AgentHandler) normalizeAgentChannelIDs(ctx context.Context, w http.ResponseWriter, orgID uuid.UUID, raw *[]string) ([]uuid.UUID, bool) {
	if raw == nil {
		return nil, true
	}
	ids := make([]uuid.UUID, 0, len(*raw))
	seen := map[uuid.UUID]bool{}
	for _, value := range *raw {
		id, err := uuid.Parse(value)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "channel_ids must contain only uuids"})
			return nil, false
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return ids, true
	}
	var count int64
	if err := h.db.WithContext(ctx).
		Model(&model.Channel{}).
		Where("org_id = ? AND id IN ? AND archived_at IS NULL", orgID, ids).
		Count(&count).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to validate channel_ids"})
		return nil, false
	}
	if count != int64(len(ids)) {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "channel_ids must belong to active channels in this org"})
		return nil, false
	}
	return ids, true
}

func (h *AgentHandler) replaceAgentChannelsTx(tx *gorm.DB, orgID, agentID uuid.UUID, channelIDs []uuid.UUID) error {
	if err := tx.Where("org_id = ? AND agent_id = ?", orgID, agentID).Delete(&model.AgentChannel{}).Error; err != nil {
		return fmt.Errorf("delete agent channels: %w", err)
	}
	if len(channelIDs) == 0 {
		return nil
	}
	rows := make([]model.AgentChannel, len(channelIDs))
	for i, channelID := range channelIDs {
		rows[i] = model.AgentChannel{OrgID: orgID, AgentID: agentID, ChannelID: channelID}
	}
	if err := tx.Create(&rows).Error; err != nil {
		return fmt.Errorf("create agent channels: %w", err)
	}
	return nil
}

func (h *AgentHandler) loadAgentChannelIDs(ctx context.Context, orgID uuid.UUID, agentIDs []uuid.UUID) map[uuid.UUID][]string {
	out := make(map[uuid.UUID][]string, len(agentIDs))
	if len(agentIDs) == 0 {
		return out
	}
	var rows []model.AgentChannel
	_ = h.db.WithContext(ctx).
		Where("org_id = ? AND agent_id IN ?", orgID, agentIDs).
		Order("created_at ASC, channel_id ASC").
		Find(&rows).Error
	for _, row := range rows {
		out[row.AgentID] = append(out[row.AgentID], row.ChannelID.String())
	}
	return out
}

func agentAllowedInChannel(ctx context.Context, db *gorm.DB, orgID, agentID, channelID uuid.UUID) bool {
	var total int64
	if err := db.WithContext(ctx).
		Model(&model.AgentChannel{}).
		Where("org_id = ? AND agent_id = ?", orgID, agentID).
		Count(&total).Error; err != nil {
		return false
	}
	if total == 0 {
		return true
	}
	var allowed int64
	if err := db.WithContext(ctx).
		Model(&model.AgentChannel{}).
		Where("org_id = ? AND agent_id = ? AND channel_id = ?", orgID, agentID, channelID).
		Count(&allowed).Error; err != nil {
		return false
	}
	return allowed > 0
}
