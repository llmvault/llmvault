package tasks

import (
	"context"
	"time"

	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sandbox"
)

// autoSleepIdleThreshold is how long a session must have gone without events
// (and be off the agent's turn) before its sandbox is put to sleep.
const autoSleepIdleThreshold = 5 * time.Minute

// SandboxAutoSleepHandler sleeps sandboxes whose sessions are idle and have not
// received events within autoSleepIdleThreshold. It stops them at the control
// plane and marks them 'stopped' locally; they wake transparently on the next
// request (and the mark-running path flips their status back).
type SandboxAutoSleepHandler struct {
	db           *gorm.DB
	orchestrator *sandbox.Orchestrator
}

func NewSandboxAutoSleepHandler(db *gorm.DB, orchestrator *sandbox.Orchestrator) *SandboxAutoSleepHandler {
	return &SandboxAutoSleepHandler{db: db, orchestrator: orchestrator}
}

func (h *SandboxAutoSleepHandler) Handle(ctx context.Context, _ *asynq.Task) error {
	cutoff := time.Now().Add(-autoSleepIdleThreshold)

	var sandboxes []model.Sandbox
	if err := h.db.WithContext(ctx).
		Distinct("sandboxes.*").
		Joins("JOIN sessions ON sessions.sandbox_id = sandboxes.id").
		Joins("LEFT JOIN LATERAL (SELECT max(event_at) AS last_event FROM session_events se WHERE se.session_id = sessions.id) ev ON TRUE").
		Where("sessions.agent_turn_status = ?", model.SessionAgentTurnIdle).
		Where("sandboxes.status = ?", string(sandbox.StatusRunning)).
		Where("sandboxes.external_id <> ''").
		Where("COALESCE(ev.last_event, sessions.created_at) < ?", cutoff).
		Find(&sandboxes).Error; err != nil {
		return err
	}

	slept := 0
	for i := range sandboxes {
		if err := h.orchestrator.SleepSandbox(ctx, &sandboxes[i]); err != nil {
			logging.FromContext(ctx).WarnContext(ctx, "auto-sleep: failed to sleep sandbox",
				"sandbox_id", sandboxes[i].ID, "error", err)
			continue
		}
		slept++
	}
	if len(sandboxes) > 0 {
		logging.FromContext(ctx).InfoContext(ctx, "auto-sleep sweep",
			"candidates", len(sandboxes), "slept", slept)
	}
	return nil
}
