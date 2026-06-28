package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/slackapp"
	"github.com/usehivy/hivy/internal/slackworkflow"
	"github.com/usehivy/hivy/internal/tasks"
)

func (h *NangoWebhookHandler) handleSlackReactionForward(w http.ResponseWriter, r *http.Request, wh *nangoWebhook, wctx *webhookContext, event slackapp.ReactionAddedEvent, fields map[string]any) {
	ctx := r.Context()
	addSlackReactionFields(fields, event)
	if event.ItemType != "message" {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
		return
	}
	channel, ok, err := h.findSlackReactionChannel(ctx, wctx.connection, event.ItemChannel)
	if err != nil {
		fields["stage"] = "find_channel"
		logging.CaptureWithFields(ctx, fmt.Errorf("slack reaction channel lookup: %w", err), fields)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to accept slack reaction"})
		return
	}
	if !ok {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
		return
	}
	trigger, ok, err := h.findSlackReactionTrigger(ctx, wctx.connection, channel.ID, event.Reaction)
	if err != nil {
		fields["stage"] = "find_trigger"
		logging.CaptureWithFields(ctx, fmt.Errorf("slack reaction trigger lookup: %w", err), fields)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to accept slack reaction"})
		return
	}
	if !ok {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
		return
	}
	fields["trigger_id"] = trigger.ID.String()
	claim, err := slackworkflow.ClaimReactionTrigger(ctx, h.db, wctx.connection.OrgID, wctx.connection.ID, trigger.ID, channel.ID, event)
	if err != nil {
		fields["stage"] = "claim_reaction"
		logging.CaptureWithFields(ctx, fmt.Errorf("slack reaction claim: %w", err), fields)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to accept slack reaction"})
		return
	}
	fields["slack_thread_event_id"] = claim.Event.ID.String()
	fields["accepted"] = claim.Accepted
	fields["duplicate"] = claim.Duplicate
	if claim.Duplicate || !claim.Accepted {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
		return
	}
	if err := tasks.EnqueueSlackReactionTrigger(ctx, h.enqueuer, claim.Event.ID, event.EventID); err != nil {
		fields["stage"] = "enqueue"
		logging.CaptureWithFields(ctx, fmt.Errorf("slack reaction enqueue: %w", err), fields)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to enqueue slack reaction"})
		return
	}
	if err := slackworkflow.MarkEnqueued(ctx, h.db, claim.Event.ID); err != nil {
		fields["stage"] = "mark_enqueued"
		logging.CaptureWithFields(ctx, err, fields)
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "accepted"})
}

func (h *NangoWebhookHandler) findSlackReactionChannel(ctx context.Context, conn *model.Connection, slackChannelID string) (model.Channel, bool, error) {
	var channel model.Channel
	err := h.db.WithContext(ctx).
		Where("org_id = ? AND origin = ? AND external_provider = ?", conn.OrgID, "external", slackapp.Provider).
		Where("external_connection_id = ? AND external_resource_type = ?", conn.ID, "slack_channel").
		Where("external_resource_key = ? AND archived_at IS NULL", slackChannelID).
		First(&channel).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.Channel{}, false, nil
	}
	if err != nil {
		return model.Channel{}, false, err
	}
	return channel, true, nil
}

func (h *NangoWebhookHandler) findSlackReactionTrigger(ctx context.Context, conn *model.Connection, channelID uuid.UUID, reaction string) (model.AgentTrigger, bool, error) {
	var trigger model.AgentTrigger
	err := h.db.WithContext(ctx).
		Joins("JOIN agents ON agents.id = agent_triggers.agent_id").
		Where("agent_triggers.org_id = ? AND agent_triggers.connection_id = ?", conn.OrgID, conn.ID).
		Where("agent_triggers.channel_id = ? AND agent_triggers.enabled = true", channelID).
		Where("agent_triggers.trigger_key = ? AND agent_triggers.trigger_value = ?", slackapp.EventReactionAdded, normalizeTriggerValue(reaction)).
		Where("agents.status <> ?", "archived").
		Order("agent_triggers.created_at ASC").
		First(&trigger).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.AgentTrigger{}, false, nil
	}
	if err != nil {
		return model.AgentTrigger{}, false, err
	}
	return trigger, true, nil
}

func addSlackReactionFields(fields map[string]any, event slackapp.ReactionAddedEvent) {
	fields["slack_team_id"] = event.TeamID
	fields["slack_event_id"] = event.EventID
	fields["slack_event_type"] = event.EventType
	fields["slack_channel_id"] = event.ItemChannel
	fields["slack_message_ts"] = event.ItemTS
	fields["slack_sender_id"] = event.UserID
	fields["slack_reaction"] = event.Reaction
	fields["slack_item_type"] = event.ItemType
}
