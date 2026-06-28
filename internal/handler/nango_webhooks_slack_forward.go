package handler

import (
	"context"
	"fmt"
	"net/http"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/slackapp"
	"github.com/usehivy/hivy/internal/slackworkflow"
	"github.com/usehivy/hivy/internal/tasks"
)

func (h *NangoWebhookHandler) handleSlackForward(w http.ResponseWriter, r *http.Request, wh *nangoWebhook, wctx *webhookContext) {
	ctx := r.Context()
	fields := slackWebhookSentryFields("decode", wh, wctx.connection, wh.Payload)
	reaction, ok, err := slackapp.DecodeReactionAddedEvent(wh.Payload)
	if err != nil {
		fields["stage"] = "decode_reaction"
		logging.CaptureWithFields(ctx, fmt.Errorf("slack reaction webhook decode: %w", err), fields)
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
		return
	}
	if ok {
		h.handleSlackReactionForward(w, r, wh, wctx, reaction, fields)
		return
	}
	event, ok, err := slackapp.DecodeInboundEvent(wh.Payload)
	if err != nil {
		fields["stage"] = "decode"
		logging.CaptureWithFields(ctx, fmt.Errorf("slack webhook decode: %w", err), fields)
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
		return
	}
	if !ok {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
		return
	}
	addSlackEventFields(fields, event)
	claim, err := slackworkflow.ClaimInbound(ctx, h.db, wctx.connection.OrgID, wctx.connection.ID, event)
	if err != nil {
		fields["stage"] = "claim_inbound"
		logging.CaptureWithFields(ctx, fmt.Errorf("slack webhook claim inbound: %w", err), fields)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to accept slack event"})
		return
	}
	fields["slack_thread_event_id"] = claim.Event.ID.String()
	fields["accepted"] = claim.Accepted
	fields["duplicate"] = claim.Duplicate
	fields["ignore_reason"] = claim.Reason
	if claim.Duplicate || !claim.Accepted {
		logging.FromContext(ctx).InfoContext(ctx, "slack_webhook_ignored", "fields", fields)
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
		return
	}
	statusToken, statusSet := h.setSlackLoading(ctx, *wctx.connection, claim.Event, fields)
	if err := tasks.EnqueueSlackAppMention(ctx, h.enqueuer, claim.Event.ID, event.EventID); err != nil {
		fields["stage"] = "enqueue"
		logging.CaptureWithFields(ctx, fmt.Errorf("slack webhook enqueue mention: %w", err), fields)
		if statusSet {
			h.clearSlackLoading(ctx, statusToken, claim.Event, fields)
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to enqueue slack mention"})
		return
	}
	if err := slackworkflow.MarkEnqueued(ctx, h.db, claim.Event.ID); err != nil {
		fields["stage"] = "mark_enqueued"
		logging.CaptureWithFields(ctx, err, fields)
	}
	logging.FromContext(ctx).InfoContext(ctx, "slack_webhook_enqueued", "fields", fields)
	writeJSON(w, http.StatusOK, map[string]string{"status": "accepted"})
}

func (h *NangoWebhookHandler) setSlackLoading(ctx context.Context, conn model.Connection, event model.SlackThreadEvent, fields map[string]any) (string, bool) {
	token, err := h.slackLoadBotToken(ctx, conn)
	if err != nil {
		fields["stage"] = "load_bot_token"
		logging.CaptureWithFields(ctx, fmt.Errorf("slack webhook load token: %w", err), fields)
		return "", false
	}
	if err := h.slackSetStatus(ctx, token, event.SlackChannelID, event.ThreadTS); err != nil {
		fields["stage"] = "set_status"
		logging.CaptureWithFields(ctx, fmt.Errorf("slack webhook set status: %w", err), fields)
		return token, false
	}
	slackworkflow.MarkStatusSet(ctx, h.db, event.ID)
	return token, true
}

func (h *NangoWebhookHandler) clearSlackLoading(ctx context.Context, token string, event model.SlackThreadEvent, fields map[string]any) {
	if h.slackClearStatus == nil {
		return
	}
	if err := h.slackClearStatus(ctx, token, event.SlackChannelID, event.ThreadTS); err != nil {
		clearFields := make(map[string]any, len(fields)+1)
		for key, value := range fields {
			clearFields[key] = value
		}
		clearFields["stage"] = "clear_status_after_enqueue_failure"
		logging.CaptureWithFields(ctx, fmt.Errorf("slack webhook clear status: %w", err), clearFields)
	}
}

func addSlackEventFields(fields map[string]any, event slackapp.InboundEvent) {
	fields["slack_team_id"] = event.TeamID
	fields["slack_event_id"] = event.EventID
	fields["slack_event_type"] = event.EventType
	fields["slack_channel_id"] = event.ChannelID
	fields["slack_thread_ts"] = event.ThreadTS
	fields["slack_message_ts"] = event.MessageTS
	fields["slack_sender_id"] = event.UserID
	fields["slack_is_thread_reply"] = event.IsThreadReply
}
