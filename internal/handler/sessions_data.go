package handler

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/runtimeevents"
	"github.com/usehivy/hivy/internal/tasks"
)

type sessionStats struct {
	ParticipantCount int64
	EventCount       int64
	LastEvent        *time.Time
}

func (h *SessionHandler) statsForSessions(ctx context.Context, ids []uuid.UUID) map[uuid.UUID]sessionStats {
	return statsForSessionsDB(ctx, h.db, ids)
}

func statsForSessionsDB(ctx context.Context, db *gorm.DB, ids []uuid.UUID) map[uuid.UUID]sessionStats {
	out := make(map[uuid.UUID]sessionStats, len(ids))
	if len(ids) == 0 {
		return out
	}
	for _, id := range ids {
		out[id] = sessionStats{}
	}
	type participantRow struct {
		SessionID uuid.UUID
		Count     int64
	}
	var participantRows []participantRow
	_ = db.WithContext(ctx).Model(&model.SessionParticipant{}).
		Select("session_id, count(*) AS count").
		Where("session_id IN ?", ids).
		Group("session_id").
		Scan(&participantRows).Error
	for _, row := range participantRows {
		stat := out[row.SessionID]
		stat.ParticipantCount = row.Count
		out[row.SessionID] = stat
	}
	type eventRow struct {
		SessionID uuid.UUID
		Count     int64
		LastEvent *time.Time
	}
	var eventRows []eventRow
	_ = db.WithContext(ctx).Model(&model.SessionEvent{}).
		Select("session_id, count(*) AS count, max(event_at) AS last_event").
		Where("session_id IN ?", ids).
		Group("session_id").
		Scan(&eventRows).Error
	for _, row := range eventRows {
		stat := out[row.SessionID]
		stat.EventCount = row.Count
		stat.LastEvent = row.LastEvent
		out[row.SessionID] = stat
	}
	return out
}

func (h *SessionHandler) participants(ctx context.Context, sessionID uuid.UUID) []sessionParticipantResponse {
	var rows []model.SessionParticipant
	_ = h.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Order("created_at ASC").
		Find(&rows).Error
	out := make([]sessionParticipantResponse, len(rows))
	for i, row := range rows {
		out[i] = sessionParticipantResponse{
			UserID:    row.UserID.String(),
			Role:      row.Role,
			InvitedBy: formatUUIDPtr(row.InvitedBy),
			JoinedAt:  formatTimePtr(row.JoinedAt),
			CreatedAt: formatRuntimeTime(row.CreatedAt),
		}
	}
	return out
}

func sessionIDsV2(sessions []model.Session) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(sessions))
	for _, session := range sessions {
		ids = append(ids, session.ID)
	}
	return ids
}

func sessionMessageCommandPayload(text string, payload model.JSON) model.JSON {
	if payload == nil {
		payload = model.JSON{}
	} else {
		clone := model.JSON{}
		for key, value := range payload {
			clone[key] = value
		}
		payload = clone
	}
	payload["text"] = text
	return payload
}

func (h *SessionHandler) createBackendUserMessageEvent(tx *gorm.DB, session *model.Session, actor *uuid.UUID, text string, payload model.JSON, opts sessionMessageDeliveryOptions) (model.SessionEvent, bool, error) {
	if session == nil || session.ID == uuid.Nil {
		return model.SessionEvent{}, false, fmt.Errorf("session event: session is required")
	}
	clientEventID := strings.TrimSpace(opts.ClientEventID)
	if clientEventID == "" {
		if raw, ok := payload["client_event_id"].(string); ok {
			clientEventID = strings.TrimSpace(raw)
		}
	}
	eventID := backendSessionEventID(clientEventID)
	eventPayload := sessionMessageCommandPayload(text, payload)
	eventPayload["client_event_id"] = clientEventID
	if clientEventID == "" {
		eventPayload["client_event_id"] = strings.TrimPrefix(eventID, "backend:")
	}
	now := time.Now().UTC()
	event := model.SessionEvent{
		OrgID:            session.OrgID,
		SessionID:        session.ID,
		AgentID:          session.AgentID,
		SandboxID:        session.SandboxID,
		RuntimeSessionID: session.ID.String(),
		EventID:          eventID,
		EventType:        runtimeevents.EventUserMessageReceived,
		ActorUserID:      actor,
		Source:           defaultString(session.Source, "web"),
		SequenceNumber:   0,
		Durability:       "durable",
		Payload:          eventPayload,
		EventAt:          now,
	}
	result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&event)
	if result.Error != nil {
		return model.SessionEvent{}, false, fmt.Errorf("create backend user message event: %w", result.Error)
	}
	if result.RowsAffected == 1 {
		return event, true, nil
	}
	var existing model.SessionEvent
	if err := tx.Where("session_id = ? AND event_id = ?", session.ID, eventID).First(&existing).Error; err != nil {
		return model.SessionEvent{}, false, fmt.Errorf("load backend user message event: %w", err)
	}
	return existing, false, nil
}

func backendSessionEventID(clientEventID string) string {
	clientEventID = strings.TrimSpace(clientEventID)
	if clientEventID != "" {
		return "client:" + clientEventID
	}
	return "backend:" + uuid.NewString()
}

func (h *SessionHandler) sessionMessageEventQueued(tx *gorm.DB, eventID uuid.UUID) (bool, error) {
	var count int64
	if err := tx.Model(&model.SessionMessageQueue{}).
		Where("session_event_id = ? AND status <> ?", eventID, "delivered").
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("count session event queue rows: %w", err)
	}
	return count > 0, nil
}

func (h *SessionHandler) deleteCreatedSessionEvent(ctx context.Context, intent sessionMessageDeliveryIntent) error {
	if !intent.EventCreated || intent.Event == nil || intent.Event.ID == uuid.Nil {
		return nil
	}
	return h.db.WithContext(ctx).Delete(&model.SessionEvent{}, "id = ?", intent.Event.ID).Error
}

func (h *SessionHandler) enqueueSessionMessageCommand(tx *gorm.DB, session *model.Session, actor *uuid.UUID, text string, payload model.JSON, opts sessionMessageDeliveryOptions, sessionEventID *uuid.UUID) error {
	if session == nil || session.ID == uuid.Nil {
		return fmt.Errorf("session message queue: session is required")
	}
	seq, err := h.nextSessionQueueSequence(tx, session.ID)
	if err != nil {
		return err
	}
	queue := model.SessionMessageQueue{
		OrgID:           session.OrgID,
		SessionID:       session.ID,
		SessionEventID:  sessionEventID,
		ActorUserID:     actor,
		MessageText:     text,
		MessagePayload:  sessionMessageCommandPayload(text, payload),
		Model:           strings.TrimSpace(opts.Model),
		ReasoningEffort: strings.TrimSpace(opts.ReasoningEffort),
		SequenceNumber:  seq,
		Status:          "pending",
	}
	if err := tx.Create(&queue).Error; err != nil {
		return fmt.Errorf("create session message queue row: %w", err)
	}
	return nil
}

func (h *SessionHandler) nextSessionQueueSequence(tx *gorm.DB, sessionID uuid.UUID) (int64, error) {
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

func eventToResponse(event model.SessionEvent) sessionEventResponse {
	return sessionEventResponse{
		ID:             event.ID.String(),
		SessionID:      event.SessionID.String(),
		AgentID:        event.AgentID.String(),
		SandboxID:      formatUUIDPtr(event.SandboxID),
		EventID:        event.EventID,
		EventType:      event.EventType,
		ActorUserID:    formatUUIDPtr(event.ActorUserID),
		Source:         event.Source,
		SequenceNumber: event.SequenceNumber,
		RuntimeSeq:     event.RuntimeSeq,
		RuntimeEventID: event.RuntimeEventID,
		TurnID:         event.TurnID,
		SpanID:         event.SpanID,
		Durability:     event.Durability,
		Payload:        event.Payload,
		EventAt:        formatRuntimeTime(event.EventAt),
	}
}

func sessionMutationEventResponse(event *model.SessionEvent) *sessionEventResponse {
	if event == nil || event.ID == uuid.Nil {
		return nil
	}
	return ptrSessionEventResponse(eventToResponse(*event))
}

func (h *SessionHandler) enqueueSessionDelivery(ctx context.Context, sessionID uuid.UUID) error {
	if h == nil || h.enqueuer == nil {
		return nil
	}
	if err := tasks.EnqueueSessionMessageDeliver(ctx, h.enqueuer, sessionID); err != nil {
		return fmt.Errorf("enqueue session message delivery: %w", err)
	}
	return nil
}

func (h *SessionHandler) enqueueSessionName(ctx context.Context, sessionID uuid.UUID) error {
	if h == nil || h.enqueuer == nil {
		return nil
	}

	task, opts, err := tasks.NewSessionNameTask(sessionID)
	if err != nil {
		return fmt.Errorf("build session name task: %w", err)
	}
	if _, err := h.enqueuer.EnqueueContext(ctx, task, opts...); err != nil {
		if errors.Is(err, asynq.ErrDuplicateTask) {
			return nil
		}
		return fmt.Errorf("enqueue session name: %w", err)
	}
	return nil
}
