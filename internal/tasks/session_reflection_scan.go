package tasks

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/enqueue"
)

const (
	sessionReflectionIdleDelay = 2 * time.Minute
	sessionReflectionScanLimit = 100
)

type SessionReflectionScanHandler struct {
	db       *gorm.DB
	enqueuer enqueue.TaskEnqueuer
	now      func() time.Time
}

func NewSessionReflectionScanHandler(db *gorm.DB, enqueuer enqueue.TaskEnqueuer) *SessionReflectionScanHandler {
	return &SessionReflectionScanHandler{db: db, enqueuer: enqueuer}
}

func (h *SessionReflectionScanHandler) Handle(ctx context.Context, _ *asynq.Task) error {
	if h == nil || h.db == nil || h.enqueuer == nil {
		return nil
	}
	now := time.Now().UTC()
	if h.now != nil {
		now = h.now().UTC()
	}
	ids, err := h.eligibleSessionIDs(ctx, now, sessionReflectionScanLimit)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := EnqueueSessionReflection(ctx, h.enqueuer, id); err != nil {
			return fmt.Errorf("enqueue session reflection: %w", err)
		}
	}
	return nil
}

func (h *SessionReflectionScanHandler) eligibleSessionIDs(ctx context.Context, now time.Time, limit int) ([]uuid.UUID, error) {
	if limit <= 0 {
		limit = sessionReflectionScanLimit
	}
	type row struct {
		SessionID uuid.UUID
	}
	var rows []row
	err := h.db.WithContext(ctx).Raw(`
SELECT s.id AS session_id
FROM sessions s
JOIN LATERAL (
	SELECT id, event_at
	FROM session_events
	WHERE session_id = s.id
		AND (durability = 'durable' OR durability = '')
	ORDER BY event_at DESC, id DESC
	LIMIT 1
) latest ON true
LEFT JOIN session_reflection_states st ON st.session_id = s.id
WHERE s.status = 'active'
	AND s.agent_turn_status = 'idle'
	AND latest.event_at <= ?
	AND (st.locked_until IS NULL OR st.locked_until <= ?)
	AND (
		st.session_id IS NULL
		OR st.last_reflected_event_at IS NULL
		OR latest.event_at > st.last_reflected_event_at
		OR (latest.event_at = st.last_reflected_event_at AND latest.id > st.last_reflected_event_id)
	)
ORDER BY latest.event_at ASC
LIMIT ?`, now.Add(-sessionReflectionIdleDelay), now, limit).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("scan reflection-eligible sessions: %w", err)
	}
	out := make([]uuid.UUID, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.SessionID)
	}
	return out, nil
}
