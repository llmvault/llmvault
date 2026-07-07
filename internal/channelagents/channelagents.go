// Package channelagents is the single source of truth for which agents may run
// in a channel. A channel has a set of assigned agents (the channel_agents join
// table); channels.default_agent_id is always one of them. Invoking an agent in
// a channel it is not assigned to is forbidden at every session-creation
// chokepoint — the ONE exemption is system-kind channels (used by triggers and
// schedules), where any org agent may run. That exemption lives in Assigned so
// every caller inherits it uniformly.
package channelagents

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

// Sentinel errors returned by Assign/Unassign so callers (HTTP handlers) can map
// them to status codes without string-matching.
var (
	// ErrAgentNotFound means the agent does not exist in the org.
	ErrAgentNotFound = errors.New("agent not found")
	// ErrAgentArchived means the agent exists but is archived and cannot be
	// assigned.
	ErrAgentArchived = errors.New("agent is archived")
	// ErrDefaultAgent means the caller tried to unassign the channel's current
	// default agent; the default must be changed first.
	ErrDefaultAgent = errors.New("cannot unassign the channel's default agent")
	// ErrChannelNotFound means the channel does not exist in the org.
	ErrChannelNotFound = errors.New("channel not found")
)

// Assigned reports whether agentID may run in channelID. It is true when an
// explicit assignment row exists OR the channel is system-kind (the exemption
// that lets triggers/schedules run any org agent in the shared system channel).
func Assigned(ctx context.Context, db *gorm.DB, orgID, channelID, agentID uuid.UUID) (bool, error) {
	var channel model.Channel
	err := db.WithContext(ctx).
		Select("id", "kind").
		Where("id = ? AND org_id = ?", channelID, orgID).
		First(&channel).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("load channel: %w", err)
	}
	if channel.Kind == "system" {
		return true, nil
	}
	var count int64
	if err := db.WithContext(ctx).
		Model(&model.ChannelAgent{}).
		Where("org_id = ? AND channel_id = ? AND agent_id = ?", orgID, channelID, agentID).
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("count channel agent: %w", err)
	}
	return count > 0, nil
}

// IsAssignedRow reports whether an explicit assignment row exists, ignoring the
// system-kind exemption. HTTP handlers use it to detect duplicate POSTs (409)
// where the exemption is irrelevant.
func IsAssignedRow(ctx context.Context, db *gorm.DB, orgID, channelID, agentID uuid.UUID) (bool, error) {
	var count int64
	if err := db.WithContext(ctx).
		Model(&model.ChannelAgent{}).
		Where("org_id = ? AND channel_id = ? AND agent_id = ?", orgID, channelID, agentID).
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("count channel agent: %w", err)
	}
	return count > 0, nil
}

// ListAssigned returns the channel's assigned, non-archived agents in a stable
// order (assignment created_at ASC, then agent id) so API responses are
// deterministic.
func ListAssigned(ctx context.Context, db *gorm.DB, orgID, channelID uuid.UUID) ([]model.Agent, error) {
	var agents []model.Agent
	err := db.WithContext(ctx).
		Model(&model.Agent{}).
		Joins("JOIN channel_agents ON channel_agents.agent_id = agents.id").
		Where("channel_agents.org_id = ? AND channel_agents.channel_id = ?", orgID, channelID).
		Where("agents.status <> ?", "archived").
		Order("channel_agents.created_at ASC, agents.id ASC").
		Preload("AgentCatalog").
		Find(&agents).Error
	if err != nil {
		return nil, fmt.Errorf("list assigned agents: %w", err)
	}
	return agents, nil
}
