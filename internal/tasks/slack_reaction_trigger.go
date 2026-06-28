package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	slacksdk "github.com/slack-go/slack"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/nango"
	"github.com/usehivy/hivy/internal/sandbox"
	"github.com/usehivy/hivy/internal/slackapp"
	"github.com/usehivy/hivy/internal/slackworkflow"
)

type SlackReactionTriggerHandler struct {
	*SlackAppMentionHandler
}

func NewSlackReactionTriggerHandler(db *gorm.DB, orchestrator *sandbox.Orchestrator, compileDeps agentruntime.CompileDeps, enq enqueue.TaskEnqueuer, nangoClient *nango.Client, ensurer OrgHivyAgentEnsurer) *SlackReactionTriggerHandler {
	return &SlackReactionTriggerHandler{
		SlackAppMentionHandler: NewSlackAppMentionHandler(db, orchestrator, compileDeps, enq, nangoClient, ensurer),
	}
}

func (h *SlackReactionTriggerHandler) Handle(ctx context.Context, task *asynq.Task) error {
	var payload SlackReactionTriggerPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal slack reaction trigger payload: %w", err)
	}
	row, err := h.loadEvent(ctx, payload.SlackThreadEventID)
	if err != nil {
		return err
	}
	ctx = logging.WithAttrs(ctx,
		"slack_thread_event_id", row.ID.String(),
		"connection_id", row.ConnectionID.String(),
		"org_id", row.OrgID.String(),
		"trigger_id", uuidPtrString(row.TriggerID),
		"slack_event_id", row.EventID,
		"slack_channel_id", row.SlackChannelID,
		"slack_thread_ts", row.ThreadTS,
	)
	if row.Status == model.SlackThreadEventStatusCompleted {
		return nil
	}
	if err := slackworkflow.MarkProcessing(ctx, h.db, row.ID); err != nil {
		return err
	}
	if err := h.processReaction(ctx, &row); err != nil {
		slackworkflow.MarkFailed(ctx, h.db, row.ID, err)
		logging.CaptureWithFields(ctx, fmt.Errorf("slack reaction trigger pipeline: %w", err), slackErrorFields(row, "process_reaction"))
		return err
	}
	return nil
}

func (h *SlackReactionTriggerHandler) processReaction(ctx context.Context, row *model.SlackThreadEvent) error {
	trigger, err := h.loadReactionTrigger(ctx, row)
	if err != nil {
		return err
	}
	token, client, err := h.slackClient(ctx, row.Connection)
	if err != nil {
		return err
	}
	messageContext, err := slackapp.FetchReactionMessageContext(ctx, client, row.SlackChannelID, row.MessageTS)
	if err != nil {
		return err
	}
	text := slackReactionAutomationText(trigger, *row, messageContext)
	if err := h.updateReactionContext(ctx, row, messageContext.ThreadTS, text); err != nil {
		return err
	}
	return h.SlackAppMentionHandler.processWithSlack(ctx, row, token, client)
}

func (h *SlackReactionTriggerHandler) loadReactionTrigger(ctx context.Context, row *model.SlackThreadEvent) (model.AgentTrigger, error) {
	if row.TriggerID == nil || *row.TriggerID == uuid.Nil {
		return model.AgentTrigger{}, fmt.Errorf("slack reaction event is missing trigger_id")
	}
	var trigger model.AgentTrigger
	err := h.db.WithContext(ctx).
		Preload("Agent").
		Where("id = ? AND org_id = ? AND enabled = true", *row.TriggerID, row.OrgID).
		Where("trigger_key = ?", slackapp.EventReactionAdded).
		First(&trigger).Error
	if err != nil {
		return model.AgentTrigger{}, fmt.Errorf("load slack reaction trigger: %w", err)
	}
	return trigger, nil
}

func (h *SlackReactionTriggerHandler) updateReactionContext(ctx context.Context, row *model.SlackThreadEvent, threadTS, text string) error {
	threadTS = strings.TrimSpace(threadTS)
	if threadTS == "" {
		threadTS = row.MessageTS
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return fmt.Errorf("slack reaction automation text is empty")
	}
	now := time.Now().UTC()
	if err := h.db.WithContext(ctx).Model(&model.SlackThreadEvent{}).Where("id = ?", row.ID).Updates(map[string]any{
		"thread_ts":  threadTS,
		"text":       text,
		"updated_at": now,
	}).Error; err != nil {
		return fmt.Errorf("update slack reaction context: %w", err)
	}
	row.ThreadTS = threadTS
	row.Text = text
	return nil
}

func slackReactionAutomationText(trigger model.AgentTrigger, row model.SlackThreadEvent, context slackapp.ReactionMessageContext) string {
	reaction := rawSlackReaction(row.Raw)
	if reaction == "" {
		reaction = trigger.TriggerValue
	}
	var b strings.Builder
	b.WriteString("Slack automation triggered by reaction :")
	b.WriteString(reaction)
	b.WriteString(":.\n\nAutomation instructions:\n")
	if strings.TrimSpace(trigger.Instructions) == "" {
		b.WriteString("No additional instructions were configured.")
	} else {
		b.WriteString(strings.TrimSpace(trigger.Instructions))
	}
	b.WriteString("\n\nReaction:\n")
	b.WriteString("- emoji: :")
	b.WriteString(reaction)
	b.WriteString(":\n")
	writeAutomationLine(&b, "reacted_by", slackUserTag(row.SenderID))
	writeAutomationLine(&b, "item_user", slackUserTag(rawSlackItemUser(row.Raw)))
	writeAutomationLine(&b, "channel_id", row.SlackChannelID)
	writeAutomationLine(&b, "message_ts", row.MessageTS)
	writeAutomationLine(&b, "thread_ts", context.ThreadTS)
	b.WriteString("\nReacted message:\n")
	b.WriteString(slackMessageLine(context.Message))
	if len(context.ThreadMessages) > 0 {
		b.WriteString("\n\nThread context:\n")
		for _, message := range context.ThreadMessages {
			b.WriteString("- ")
			b.WriteString(slackMessageLine(message))
			b.WriteString("\n")
		}
	}
	return strings.TrimSpace(b.String())
}

func writeAutomationLine(b *strings.Builder, key, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	b.WriteString("- ")
	b.WriteString(key)
	b.WriteString(": ")
	b.WriteString(value)
	b.WriteString("\n")
}

func slackMessageLine(message slacksdk.Message) string {
	sender := slackMessageSender(message)
	text := strings.TrimSpace(message.Text)
	if text == "" {
		text = "(no text)"
	}
	ts := strings.TrimSpace(message.Timestamp)
	if ts == "" {
		return sender + ": " + text
	}
	return sender + " [" + ts + "]: " + text
}

func slackMessageSender(message slacksdk.Message) string {
	if strings.TrimSpace(message.User) != "" {
		return slackUserTag(message.User)
	}
	if strings.TrimSpace(message.Username) != "" {
		return strings.TrimSpace(message.Username)
	}
	if strings.TrimSpace(message.BotID) != "" {
		return "bot:" + strings.TrimSpace(message.BotID)
	}
	return "unknown"
}

func slackUserTag(userID string) string {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return ""
	}
	return "<@" + userID + ">"
}

func rawSlackReaction(raw model.JSON) string {
	return rawSlackEventString(raw, "reaction")
}

func rawSlackItemUser(raw model.JSON) string {
	return rawSlackEventString(raw, "item_user")
}

func rawSlackEventString(raw model.JSON, key string) string {
	event, _ := raw["event"].(map[string]any)
	value, _ := event[key].(string)
	return strings.TrimSpace(value)
}
