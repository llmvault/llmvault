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

func resolveScheduleChannel(ctx context.Context, db *gorm.DB, orgID, agentID uuid.UUID, raw string) (string, error) {
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

func agentAllowedInScheduleChannel(ctx context.Context, db *gorm.DB, orgID, agentID, channelID uuid.UUID) (bool, error) {
	var total int64
	if err := db.WithContext(ctx).
		Model(&model.AgentChannel{}).
		Where("org_id = ? AND agent_id = ?", orgID, agentID).
		Count(&total).Error; err != nil {
		return false, fmt.Errorf("validate agent channel access: %w", err)
	}
	if total == 0 {
		return true, nil
	}
	var allowed int64
	if err := db.WithContext(ctx).
		Model(&model.AgentChannel{}).
		Where("org_id = ? AND agent_id = ? AND channel_id = ?", orgID, agentID, channelID).
		Count(&allowed).Error; err != nil {
		return false, fmt.Errorf("validate agent channel access: %w", err)
	}
	return allowed > 0, nil
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
