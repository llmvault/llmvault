package tasks

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/slackapp"
)

func (h *SlackAppMentionHandler) resolveSlackThreadContinuation(ctx context.Context, row *model.SlackThreadEvent) error {
	if row == nil || row.EventType == slackapp.EventReactionAdded || (row.SessionID != nil && *row.SessionID != uuid.Nil) {
		return nil
	}
	threadTS := strings.TrimSpace(row.ThreadTS)
	if threadTS == "" {
		return nil
	}
	outbound, ok, err := h.latestSlackThreadOutbound(ctx, *row)
	if err != nil {
		return err
	}
	if !ok || outbound.SessionID == nil || *outbound.SessionID == uuid.Nil {
		return nil
	}
	now := time.Now().UTC()
	updates := map[string]any{
		"session_id":          outbound.SessionID,
		"session_resolved_at": now,
		"updated_at":          now,
	}
	row.SessionID = outbound.SessionID
	if outbound.ChannelID != nil && *outbound.ChannelID != uuid.Nil {
		row.ChannelID = outbound.ChannelID
		updates["channel_id"] = outbound.ChannelID
		updates["channel_resolved_at"] = now
	}
	if outbound.TriggerID != nil && *outbound.TriggerID != uuid.Nil {
		row.TriggerID = outbound.TriggerID
		updates["trigger_id"] = outbound.TriggerID
	}
	if err := h.db.WithContext(ctx).Model(&model.SlackThreadEvent{}).
		Where("id = ?", row.ID).
		Updates(updates).Error; err != nil {
		return fmt.Errorf("record slack thread continuation: %w", err)
	}
	return nil
}

func (h *SlackAppMentionHandler) latestSlackThreadOutbound(ctx context.Context, row model.SlackThreadEvent) (model.SlackThreadEvent, bool, error) {
	var outbound model.SlackThreadEvent
	err := h.db.WithContext(ctx).
		Where("org_id = ? AND connection_id = ?", row.OrgID, row.ConnectionID).
		Where("slack_channel_id = ? AND thread_ts = ? AND direction = ?", row.SlackChannelID, row.ThreadTS, model.SlackThreadEventDirectionOutbound).
		Where("status IN ? AND session_id IS NOT NULL", []string{model.SlackThreadEventStatusSent, model.SlackThreadEventStatusCompleted}).
		Order("message_at DESC, created_at DESC").
		First(&outbound).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.SlackThreadEvent{}, false, nil
	}
	if err != nil {
		return model.SlackThreadEvent{}, false, fmt.Errorf("load latest slack thread outbound: %w", err)
	}
	return outbound, true, nil
}
