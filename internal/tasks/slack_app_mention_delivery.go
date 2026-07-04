package tasks

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/runtimeevents"
	"github.com/usehivy/hivy/internal/slackworkflow"
)

type slackMessageIntent struct {
	Session model.Session
	Event   model.SessionEvent
	Command SessionMessageCommand
	QueueID *uuid.UUID
	Queued  bool
	Direct  bool
}

func (h *SlackAppMentionHandler) prepareSlackMessageIntent(ctx context.Context, row *model.SlackThreadEvent, base model.Session) (slackMessageIntent, error) {
	var intent slackMessageIntent
	err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var session model.Session
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&session, "id = ?", base.ID).Error; err != nil {
			return fmt.Errorf("lock slack session: %w", err)
		}
		event, err := h.ensureSlackSessionEvent(tx, row, session)
		if err != nil {
			return err
		}
		intent = slackMessageIntent{
			Session: session,
			Event:   event,
			Command: slackCommandFromEvent(event),
		}
		if row.SessionEventID == nil {
			eventID := event.ID
			row.SessionEventID = &eventID
		}
		if row.SessionID == nil {
			sessionID := session.ID
			row.SessionID = &sessionID
		}
		if session.AgentTurnStatus == model.SessionAgentTurnActive {
			queue, err := h.ensureSlackQueueRow(tx, row, session, event)
			if err != nil {
				return err
			}
			queueID := queue.ID
			intent.QueueID = &queueID
			intent.Queued = true
			return nil
		}
		now := time.Now().UTC()
		if err := tx.Model(&model.Session{}).Where("id = ?", session.ID).Updates(map[string]any{
			"agent_turn_status":       model.SessionAgentTurnActive,
			"agent_turn_id":           "",
			"agent_stream_id":         "",
			"agent_turn_started_at":   now,
			"agent_turn_last_outcome": "",
			"updated_at":              now,
		}).Error; err != nil {
			return fmt.Errorf("mark slack session turn active: %w", err)
		}
		session.AgentTurnStatus = model.SessionAgentTurnActive
		session.AgentTurnStartedAt = &now
		intent.Session = session
		intent.Direct = true
		return tx.Model(&model.SlackThreadEvent{}).Where("id = ?", row.ID).Updates(map[string]any{
			"session_id":       row.SessionID,
			"session_event_id": row.SessionEventID,
			"updated_at":       now,
		}).Error
	})
	return intent, err
}

func (h *SlackAppMentionHandler) ensureSlackSessionEvent(tx *gorm.DB, row *model.SlackThreadEvent, session model.Session) (model.SessionEvent, error) {
	eventID := "slack:" + row.EventID
	payload := model.JSON{
		"text": slackAgentText(*row),
		"slack": map[string]any{
			"team_id":       row.TeamID,
			"channel_id":    row.SlackChannelID,
			"thread_ts":     row.ThreadTS,
			"message_ts":    row.MessageTS,
			"sender_id":     row.SenderID,
			"sender_tag":    slackSenderTag(row.SenderID),
			"user_name":     payloadString(row.Raw, "user_name"),
			"display_name":  payloadString(row.Raw, "display_name"),
			"event_id":      row.EventID,
			"event_type":    row.EventType,
			"connection_id": row.ConnectionID.String(),
			"trigger_id":    uuidPtrString(row.TriggerID),
		},
	}
	event := model.SessionEvent{
		OrgID:            session.OrgID,
		SessionID:        session.ID,
		AgentID:          session.AgentID,
		SandboxID:        session.SandboxID,
		RuntimeSessionID: session.ID.String(),
		EventID:          eventID,
		EventType:        runtimeevents.EventUserMessageReceived,
		Source:           model.SessionSourceExternal,
		SequenceNumber:   0,
		Durability:       "durable",
		Payload:          payload,
		EventAt:          time.Now().UTC(),
	}
	result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&event)
	if result.Error != nil {
		return model.SessionEvent{}, fmt.Errorf("create slack session event: %w", result.Error)
	}
	if result.RowsAffected == 1 {
		return event, nil
	}
	if err := tx.Where("session_id = ? AND event_id = ?", session.ID, eventID).First(&event).Error; err != nil {
		return model.SessionEvent{}, fmt.Errorf("load slack session event: %w", err)
	}
	return event, nil
}

func (h *SlackAppMentionHandler) ensureSlackQueueRow(tx *gorm.DB, row *model.SlackThreadEvent, session model.Session, event model.SessionEvent) (model.SessionMessageQueue, error) {
	var existing model.SessionMessageQueue
	err := tx.Where("session_id = ? AND session_event_id = ?", session.ID, event.ID).First(&existing).Error
	if err == nil {
		return existing, h.updateSlackQueueLink(tx, row, session.ID, event.ID, existing.ID)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return model.SessionMessageQueue{}, fmt.Errorf("load slack queue row: %w", err)
	}
	seq, err := nextSessionQueueSequence(tx, session.ID)
	if err != nil {
		return model.SessionMessageQueue{}, err
	}
	eventID := event.ID
	queue := model.SessionMessageQueue{
		OrgID:           session.OrgID,
		SessionID:       session.ID,
		SessionEventID:  &eventID,
		MessageText:     slackAgentText(*row),
		MessagePayload:  event.Payload,
		SequenceNumber:  seq,
		Status:          "pending",
		Model:           session.Model,
		ReasoningEffort: session.ReasoningEffort,
	}
	if err := tx.Create(&queue).Error; err != nil {
		return model.SessionMessageQueue{}, fmt.Errorf("create slack queue row: %w", err)
	}
	return queue, h.updateSlackQueueLink(tx, row, session.ID, event.ID, queue.ID)
}

func (h *SlackAppMentionHandler) updateSlackQueueLink(tx *gorm.DB, row *model.SlackThreadEvent, sessionID, eventID, queueID uuid.UUID) error {
	now := time.Now().UTC()
	return tx.Model(&model.SlackThreadEvent{}).Where("id = ?", row.ID).Updates(map[string]any{
		"session_id":               &sessionID,
		"session_event_id":         &eventID,
		"session_message_queue_id": &queueID,
		"updated_at":               now,
	}).Error
}

func (h *SlackAppMentionHandler) deliverSlackIntent(ctx context.Context, row *model.SlackThreadEvent, session model.Session, intent slackMessageIntent) (*agentruntime.HTTPMessageResponse, error) {
	dispatcher := NewSessionMessageDeliverHandler(h.db, h.orchestrator, h.compileDeps, h.enqueuer)
	if intent.Queued {
		if err := EnqueueSessionMessageDeliver(ctx, h.enqueuer, session.ID); err != nil {
			return nil, err
		}
		return h.waitQueuedSlackDelivery(ctx, row.ID, *intent.QueueID)
	}
	resp, err := dispatcher.DeliverCommand(ctx, intent.Session, intent.Command)
	if err != nil {
		_ = dispatcher.releaseSessionTurn(context.WithoutCancel(ctx), session.ID)
		return nil, err
	}
	if err := h.recordDirectSlackDelivery(ctx, session.ID, resp); err != nil {
		return nil, err
	}
	_ = slackworkflow.RecordRuntimePosted(ctx, h.db, row.ID, resp.StreamID, resp.TurnID)
	return resp, nil
}

func (h *SlackAppMentionHandler) recordDirectSlackDelivery(ctx context.Context, sessionID uuid.UUID, delivery *agentruntime.HTTPMessageResponse) error {
	updates := map[string]any{
		"agent_turn_status":       model.SessionAgentTurnActive,
		"agent_turn_last_outcome": "",
	}
	if delivery != nil {
		updates["agent_turn_id"] = strings.TrimSpace(delivery.TurnID)
		updates["agent_stream_id"] = strings.TrimSpace(delivery.StreamID)
	}
	return h.db.WithContext(ctx).Model(&model.Session{}).Where("id = ?", sessionID).Updates(updates).Error
}

func slackCommandFromEvent(event model.SessionEvent) SessionMessageCommand {
	eventID := event.ID
	text, _ := event.Payload["text"].(string)
	return SessionMessageCommand{EventID: &eventID, Text: text, Payload: event.Payload}
}

func slackAgentText(row model.SlackThreadEvent) string {
	text := strings.TrimSpace(row.Text)
	if text == "" {
		text = strings.TrimSpace(row.MessageTS)
	}
	if row.EventType == "reaction_added" {
		return text
	}
	var b strings.Builder
	b.WriteString("Slack message:\n\n")
	if strings.TrimSpace(row.SenderID) != "" {
		b.WriteString("sender_id: ")
		b.WriteString(strings.TrimSpace(row.SenderID))
		b.WriteString("\nsender_tag: ")
		b.WriteString(slackSenderTag(row.SenderID))
		b.WriteString("\n\n")
	}
	b.WriteString("Message:\n")
	b.WriteString(text)
	return b.String()
}

func slackSenderTag(senderID string) string {
	senderID = strings.TrimSpace(senderID)
	if senderID == "" {
		return ""
	}
	return "<@" + senderID + ">"
}

// nextSessionQueueSequence returns the next per-session message-queue sequence.
// Callers must hold a lock on the session row so the (session_id,
// sequence_number) allocation is race-free against concurrent enqueues.
func nextSessionQueueSequence(tx *gorm.DB, sessionID uuid.UUID) (int64, error) {
	var maxSeq int64
	err := tx.Model(&model.SessionMessageQueue{}).
		Where("session_id = ?", sessionID).
		Select("COALESCE(MAX(sequence_number), 0)").
		Scan(&maxSeq).Error
	if err != nil {
		return 0, err
	}
	return maxSeq + 1, nil
}
