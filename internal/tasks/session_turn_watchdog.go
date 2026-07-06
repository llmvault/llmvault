package tasks

import (
	"context"
	"time"

	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

// stuckTurnThreshold must exceed the longest silent gap inside a real turn: a
// live turn streams events, so this much total silence means the terminal turn
// event was lost (runtime crash / killed sandbox / dropped webhook), which would
// otherwise strand the turn 'active' forever and exempt its sandbox from
// auto-sleep.
const stuckTurnThreshold = 30 * time.Minute

// SessionTurnWatchdogHandler resets sessions stuck in an 'active' turn. A stuck
// turn never flips back to idle (that needs a turn_completed/failed/interrupted
// event), so resetting it both frees the sandbox and surfaces the leak.
type SessionTurnWatchdogHandler struct {
	db *gorm.DB
}

func NewSessionTurnWatchdogHandler(db *gorm.DB) *SessionTurnWatchdogHandler {
	return &SessionTurnWatchdogHandler{db: db}
}

func (h *SessionTurnWatchdogHandler) Handle(ctx context.Context, _ *asynq.Task) error {
	cutoff := time.Now().Add(-stuckTurnThreshold)

	// Stuck requires BOTH the turn open past the threshold AND no event in that
	// window, so a live long turn (which keeps streaming events) is spared.
	// Mirrors the turn_failed reset in runtime_turn_state.go.
	res := h.db.WithContext(ctx).Exec(`
UPDATE sessions AS s
   SET agent_turn_status = ?,
       agent_turn_last_outcome = ?,
       agent_turn_id = '',
       agent_stream_id = '',
       agent_turn_started_at = NULL,
       updated_at = now()
 WHERE s.agent_turn_status = ?
   AND COALESCE(s.agent_turn_started_at, s.created_at) < ?
   AND COALESCE(
         (SELECT max(e.event_at) FROM session_events e WHERE e.session_id = s.id),
         s.created_at
       ) < ?`,
		model.SessionAgentTurnIdle,
		model.SessionAgentTurnOutcomeFailed,
		model.SessionAgentTurnActive,
		cutoff, cutoff)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected > 0 {
		logging.FromContext(ctx).WarnContext(ctx, "reset stuck agent turns",
			"count", res.RowsAffected, "threshold", stuckTurnThreshold.String())
	}
	return nil
}
