package channelagents

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/model"
)

// Assign idempotently assigns agentID to channelID. It rejects archived agents
// (ErrAgentArchived) and agents outside the org (ErrAgentNotFound). Re-assigning
// an already-assigned agent is a no-op, so internal callers (channel create,
// default-agent change) can call it unconditionally.
func Assign(ctx context.Context, db *gorm.DB, orgID, channelID, agentID uuid.UUID, addedBy *uuid.UUID) error {
	var agent model.Agent
	err := db.WithContext(ctx).
		Select("id", "status").
		Where("id = ? AND org_id = ?", agentID, orgID).
		First(&agent).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrAgentNotFound
	}
	if err != nil {
		return fmt.Errorf("load agent: %w", err)
	}
	if agent.Status == "archived" {
		return ErrAgentArchived
	}
	row := model.ChannelAgent{
		OrgID:     orgID,
		ChannelID: channelID,
		AgentID:   agentID,
		AddedBy:   addedBy,
	}
	if err := db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "channel_id"}, {Name: "agent_id"}},
			DoNothing: true,
		}).
		Create(&row).Error; err != nil {
		return fmt.Errorf("assign channel agent: %w", err)
	}
	return nil
}

// Unassign removes agentID from channelID. It refuses to remove the channel's
// current default agent (ErrDefaultAgent) — the default must be changed first so
// the "default is always assigned" invariant holds.
func Unassign(ctx context.Context, db *gorm.DB, orgID, channelID, agentID uuid.UUID) error {
	var channel model.Channel
	err := db.WithContext(ctx).
		Select("id", "default_agent_id").
		Where("id = ? AND org_id = ?", channelID, orgID).
		First(&channel).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrChannelNotFound
	}
	if err != nil {
		return fmt.Errorf("load channel: %w", err)
	}
	if channel.DefaultAgentID == agentID {
		return ErrDefaultAgent
	}
	if err := db.WithContext(ctx).
		Where("org_id = ? AND channel_id = ? AND agent_id = ?", orgID, channelID, agentID).
		Delete(&model.ChannelAgent{}).Error; err != nil {
		return fmt.Errorf("unassign channel agent: %w", err)
	}
	return nil
}
