package runtimestream

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/runtimeevents"
	"github.com/usehivy/hivy/internal/tasks"
)

func (p *Projector) processMessages(ctx context.Context, stream string, messages []redis.XMessage) error {
	if len(messages) == 0 {
		return nil
	}
	ackIDs := make([]string, 0, len(messages))
	durable := make([]Event, 0, len(messages))
	for _, msg := range messages {
		event, err := EventFromStreamValues(msg.Values)
		if err != nil {
			return fmt.Errorf("decode stream message %s: %w", msg.ID, err)
		}
		ackIDs = append(ackIDs, msg.ID)
		if event.Durability == DurabilityDurable {
			durable = append(durable, event)
		}
	}
	if len(durable) > 0 {
		if err := p.persistDurableEvents(ctx, durable); err != nil {
			return err
		}
	}
	if len(ackIDs) > 0 {
		if err := p.store.Redis().XAck(ctx, stream, ProjectorGroup, ackIDs...).Err(); err != nil {
			return fmt.Errorf("ack runtime stream messages: %w", err)
		}
	}
	return nil
}

func (p *Projector) persistDurableEvents(ctx context.Context, events []Event) error {
	rows := make([]model.SessionEvent, 0, len(events))
	for _, event := range events {
		row, err := sessionEventFromRuntimeEvent(event)
		if err != nil {
			return err
		}
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		return nil
	}
	if err := p.db.WithContext(ctx).Clauses(runtimeSeqOnConflict()).Create(&rows).Error; err != nil {
		return fmt.Errorf("insert durable runtime events: %w", err)
	}
	for _, event := range events {
		if err := p.publishCommittedEvent(ctx, event); err != nil {
			return err
		}
		if err := p.projectSessionState(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

func (p *Projector) publishCommittedEvent(ctx context.Context, event Event) error {
	sessionID, err := uuid.Parse(event.SessionID)
	if err != nil {
		return err
	}
	var stored model.SessionEvent
	if err := p.db.WithContext(ctx).
		Where("session_id = ? AND runtime_seq = ?", sessionID, event.RuntimeSeq).
		First(&stored).Error; err != nil {
		return fmt.Errorf("load committed runtime event: %w", err)
	}
	return p.store.PublishCommitted(ctx, stored)
}

func (p *Projector) projectSessionState(ctx context.Context, event Event) error {
	sessionID, err := uuid.Parse(event.SessionID)
	if err != nil {
		return err
	}
	updates := map[string]any{
		"updated_at": event.OccurredAt,
	}
	switch event.EventType {
	case runtimeevents.EventTurnStarted:
		updates["agent_turn_status"] = model.SessionAgentTurnActive
		updates["agent_turn_id"] = event.TurnID
		updates["agent_stream_id"] = defaultString(event.StreamID, event.TurnID)
		updates["agent_turn_last_outcome"] = ""
		if !event.OccurredAt.IsZero() {
			updates["agent_turn_started_at"] = event.OccurredAt
		}
	case runtimeevents.EventTurnCompleted, runtimeevents.EventFinal, runtimeevents.EventDone:
		updates["agent_turn_status"] = model.SessionAgentTurnIdle
		updates["agent_turn_id"] = ""
		updates["agent_stream_id"] = ""
		updates["agent_turn_started_at"] = nil
		updates["agent_turn_last_outcome"] = model.SessionAgentTurnOutcomeDone
	case runtimeevents.EventTurnFailed, runtimeevents.EventError, runtimeevents.EventAgentError:
		updates["agent_turn_status"] = model.SessionAgentTurnIdle
		updates["agent_turn_id"] = ""
		updates["agent_stream_id"] = ""
		updates["agent_turn_started_at"] = nil
		updates["agent_turn_last_outcome"] = model.SessionAgentTurnOutcomeFailed
	}
	if len(updates) == 1 && event.EventType != runtimeevents.EventFinal {
		return nil
	}
	if err := p.db.WithContext(ctx).Model(&model.Session{}).
		Where("id = ?", sessionID).
		Updates(updates).Error; err != nil {
		return fmt.Errorf("project session state: %w", err)
	}
	if isTerminalRuntimeEvent(event.EventType) {
		if err := p.enqueuePendingSessionDelivery(ctx, sessionID); err != nil {
			return err
		}
	}
	return nil
}

func isTerminalRuntimeEvent(eventType string) bool {
	switch eventType {
	case runtimeevents.EventTurnCompleted, runtimeevents.EventFinal, runtimeevents.EventDone,
		runtimeevents.EventTurnFailed, runtimeevents.EventError, runtimeevents.EventAgentError:
		return true
	default:
		return false
	}
}

func (p *Projector) enqueuePendingSessionDelivery(ctx context.Context, sessionID uuid.UUID) error {
	if p.enqueuer == nil {
		return nil
	}
	var pending int64
	if err := p.db.WithContext(ctx).Model(&model.SessionMessageQueue{}).
		Where("session_id = ? AND status <> ?", sessionID, "delivered").
		Count(&pending).Error; err != nil {
		return fmt.Errorf("count pending session message commands: %w", err)
	}
	if pending == 0 {
		return nil
	}
	if err := tasks.EnqueueSessionMessageDeliver(ctx, p.enqueuer, sessionID); err != nil {
		return fmt.Errorf("enqueue pending session message command: %w", err)
	}
	return nil
}
