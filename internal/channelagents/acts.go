// Package channelagents is the single source of truth for which agents may run
// in a channel. Under the team-primary model an agent is a team member: it may
// act in a channel iff the agent's team owns the channel
// (agent.team_id == channel.team_id, both non-null). There is no per-channel
// assignment table anymore.
package channelagents

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

// ActsInChannel reports whether agentID may run in channelID. It is true when
// the agent belongs to the channel's owning team. Both the channel and the
// agent are looked up scoped to orgID; a missing channel or agent yields
// (false, nil).
func ActsInChannel(ctx context.Context, db *gorm.DB, orgID, channelID, agentID uuid.UUID) (bool, error) {
	var channel model.Channel
	err := db.WithContext(ctx).
		Select("id", "team_id").
		Where("id = ? AND org_id = ?", channelID, orgID).
		First(&channel).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("load channel: %w", err)
	}
	var agent model.Agent
	err = db.WithContext(ctx).
		Select("id", "team_id").
		Where("id = ? AND org_id = ?", agentID, orgID).
		First(&agent).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("load agent: %w", err)
	}
	return agent.TeamID == channel.TeamID, nil
}
