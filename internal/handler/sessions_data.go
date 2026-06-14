package handler

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/tasks"
)

type sessionStats struct {
	ParticipantCount int64
	EventCount       int64
	LastEvent        *time.Time
}

func (h *SessionHandler) statsForSessions(ctx context.Context, ids []uuid.UUID) map[uuid.UUID]sessionStats {
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
	_ = h.db.WithContext(ctx).Model(&model.SessionParticipant{}).
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
	_ = h.db.WithContext(ctx).Model(&model.SessionEvent{}).
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

func (h *SessionHandler) createUserMessageEvent(tx *gorm.DB, session *model.Session, actor *uuid.UUID, text string, payload model.JSON) (model.SessionEvent, error) {
	seq, err := h.nextSessionSequence(tx, session.ID)
	if err != nil {
		return model.SessionEvent{}, err
	}
	if payload == nil {
		payload = model.JSON{}
	}
	payload["text"] = text
	event := model.SessionEvent{
		OrgID:            session.OrgID,
		SessionID:        session.ID,
		AgentID:          session.AgentID,
		SandboxID:        session.SandboxID,
		RuntimeSessionID: session.ID.String(),
		EventID:          "user-" + uuid.NewString(),
		EventType:        "user.message",
		ActorUserID:      actor,
		Source:           defaultString(session.Source, "web"),
		SequenceNumber:   seq,
		Payload:          payload,
		EventAt:          time.Now(),
	}
	if err := tx.Create(&event).Error; err != nil {
		return model.SessionEvent{}, err
	}
	queue := model.SessionMessageQueue{
		OrgID:          session.OrgID,
		SessionID:      session.ID,
		SessionEventID: event.ID,
		SequenceNumber: seq,
		Status:         "pending",
	}
	if err := tx.Create(&queue).Error; err != nil {
		return model.SessionEvent{}, err
	}
	return event, nil
}

func (h *SessionHandler) nextSessionSequence(tx *gorm.DB, sessionID uuid.UUID) (int64, error) {
	var maxSeq int64
	err := tx.Model(&model.SessionEvent{}).
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
		Payload:        event.Payload,
		EventAt:        formatRuntimeTime(event.EventAt),
	}
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
