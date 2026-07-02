package agentschedule

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

const systemChannelName = "system"

// ResolveScheduleChannel resolves a raw channel_id argument to a Hivy channel
// UUID string. An empty value falls back to the org's private "system"
// channel (created lazily if missing); a non-empty value must be a UUID of a
// non-archived channel in the org that the agent is allowed to use. Exported
// so other agent-facing tools (e.g. the HTTP trigger tool) share the exact
// same channel semantics as cron schedules.
func ResolveScheduleChannel(ctx context.Context, db *gorm.DB, orgID, agentID uuid.UUID, raw string) (string, error) {
	if strings.TrimSpace(raw) != "" {
		return validateScheduleChannel(ctx, db, orgID, agentID, raw)
	}
	channel, err := ensureSystemChannel(ctx, db, orgID, agentID)
	if err != nil {
		return "", err
	}
	return channel.ID.String(), nil
}

func validateScheduleChannel(ctx context.Context, db *gorm.DB, orgID, agentID uuid.UUID, raw string) (string, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil || parsed == uuid.Nil {
		return "", fmt.Errorf("channel_id must be a uuid")
	}
	var channel model.Channel
	err = db.WithContext(ctx).
		Where("id = ? AND org_id = ? AND archived_at IS NULL", parsed, orgID).
		First(&channel).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", fmt.Errorf("channel_id not found")
	}
	if err != nil {
		return "", fmt.Errorf("load channel: %w", err)
	}
	allowed, err := agentAllowedInScheduleChannel(ctx, db, orgID, agentID, channel.ID)
	if err != nil {
		return "", err
	}
	if !allowed {
		return "", fmt.Errorf("agent is not available in this channel")
	}
	return channel.ID.String(), nil
}

// RestrictedChannelIDs returns the set of channel ids the agent is limited to
// via agent_channels rows, or nil when the agent has no restrictions and may
// be used in any org channel. This is the single access rule shared by
// schedule/trigger channel validation and the list_channels MCP tool so the
// two cannot drift.
func RestrictedChannelIDs(ctx context.Context, db *gorm.DB, orgID, agentID uuid.UUID) (map[uuid.UUID]struct{}, error) {
	var rows []model.AgentChannel
	if err := db.WithContext(ctx).
		Select("channel_id").
		Where("org_id = ? AND agent_id = ?", orgID, agentID).
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("validate agent channel access: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	out := make(map[uuid.UUID]struct{}, len(rows))
	for _, row := range rows {
		out[row.ChannelID] = struct{}{}
	}
	return out, nil
}

func agentAllowedInScheduleChannel(ctx context.Context, db *gorm.DB, orgID, agentID, channelID uuid.UUID) (bool, error) {
	restricted, err := RestrictedChannelIDs(ctx, db, orgID, agentID)
	if err != nil {
		return false, err
	}
	if restricted == nil {
		return true, nil
	}
	_, ok := restricted[channelID]
	return ok, nil
}

func ensureSystemChannel(ctx context.Context, db *gorm.DB, orgID, agentID uuid.UUID) (*model.Channel, error) {
	scope := model.Channel{OrgID: orgID, Origin: "native", Name: systemChannelName}
	attrs := model.Channel{
		Description:    "System-managed jobs",
		Kind:           "system",
		Visibility:     "private",
		DefaultAgentID: agentID,
		ExternalMetadata: model.JSON{
			"source": "system",
		},
	}
	var channel model.Channel
	if err := db.WithContext(ctx).
		Where(&scope).
		Where("archived_at IS NULL").
		Attrs(attrs).
		FirstOrCreate(&channel).Error; err != nil {
		return nil, fmt.Errorf("ensure system channel: %w", err)
	}
	return &channel, nil
}
