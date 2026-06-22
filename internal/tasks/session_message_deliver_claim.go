package tasks

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/model"
)

const sessionMessageLease = 5 * time.Minute

func (h *SessionMessageDeliverHandler) claimNext(ctx context.Context, sessionID uuid.UUID) (*model.SessionMessageQueue, error) {
	var claimed model.SessionMessageQueue
	err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var session model.Session
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", sessionID).
			First(&session).Error; err != nil {
			return fmt.Errorf("lock session for message delivery: %w", err)
		}
		if session.AgentTurnStatus == model.SessionAgentTurnActive {
			return ErrSessionTurnActive
		}
		draining, err := sessionRuntimeDraining(ctx, tx, session)
		if err != nil {
			return fmt.Errorf("check session runtime drain status: %w", err)
		}
		if draining {
			return ErrSessionRuntimeDraining
		}
		var row model.SessionMessageQueue
		res := tx.Raw(`
SELECT q.*
FROM session_message_queue q
WHERE q.session_id = ?
  AND (q.status = 'pending' OR (q.status = 'processing' AND (q.leased_until IS NULL OR q.leased_until < now())))
  AND NOT EXISTS (
    SELECT 1
    FROM session_message_queue prev
    WHERE prev.session_id = q.session_id
      AND prev.sequence_number < q.sequence_number
      AND prev.status <> 'delivered'
  )
ORDER BY q.sequence_number ASC
LIMIT 1
FOR UPDATE SKIP LOCKED`, sessionID).Scan(&row)
		if res.Error != nil {
			return fmt.Errorf("claim session message row: %w", res.Error)
		}
		if res.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		leaseUntil := time.Now().Add(sessionMessageLease)
		updates := map[string]any{
			"status":        "processing",
			"attempt_count": gorm.Expr("attempt_count + 1"),
			"leased_by":     h.leaseOwner,
			"leased_until":  leaseUntil,
			"last_error":    "",
		}
		if err := tx.Model(&model.SessionMessageQueue{}).Where("id = ?", row.ID).Updates(updates).Error; err != nil {
			return fmt.Errorf("mark session message processing: %w", err)
		}
		if err := tx.Model(&model.Session{}).
			Where("id = ?", sessionID).
			Updates(map[string]any{
				"agent_turn_status":     model.SessionAgentTurnActive,
				"agent_turn_id":         "",
				"agent_stream_id":       "",
				"agent_turn_started_at": time.Now(),
			}).Error; err != nil {
			return fmt.Errorf("mark session agent turn active: %w", err)
		}
		claimed = row
		claimed.Status = "processing"
		claimed.LeasedBy = h.leaseOwner
		claimed.LeasedUntil = &leaseUntil
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := h.db.WithContext(ctx).
		Preload("Session").
		Preload("SessionEvent").
		First(&claimed, "id = ?", claimed.ID).Error; err != nil {
		return nil, fmt.Errorf("load claimed session message: %w", err)
	}
	return &claimed, nil
}
