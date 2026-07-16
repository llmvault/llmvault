package tasks

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
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

	// Agents and apps are disjoint and measure idle differently, so two queries
	// feed one sleep loop.
	agents, err := h.idleAgentSandboxes(ctx, cutoff)
	if err != nil {
		return err
	}
	apps, err := h.idleAppSandboxes(ctx, cutoff)
	if err != nil {
		return err
	}
	candidates := append(agents, apps...)

	slept := 0
	for i := range agents {
		idle, err := h.agentSandboxStillIdle(ctx, agents[i].ID, cutoff)
		if err != nil {
			logging.FromContext(ctx).WarnContext(ctx, "auto-sleep: failed to recheck agent sandbox",
				"sandbox_id", agents[i].ID, "error", err)
			continue
		}
		if !idle {
			continue
		}
		if err := h.orchestrator.SleepSandbox(ctx, &agents[i]); err != nil {
			logging.FromContext(ctx).WarnContext(ctx, "auto-sleep: failed to sleep sandbox",
				"sandbox_id", agents[i].ID, "error", err)
			continue
		}
		slept++
	}
	for i := range apps {
		idle, err := h.appSandboxStillIdle(ctx, apps[i].ID, cutoff)
		if err != nil {
			logging.FromContext(ctx).WarnContext(ctx, "auto-sleep: failed to recheck app sandbox",
				"sandbox_id", apps[i].ID, "error", err)
			continue
		}
		if !idle {
			continue
		}
		if err := h.orchestrator.SleepSandbox(ctx, &apps[i]); err != nil {
			logging.FromContext(ctx).WarnContext(ctx, "auto-sleep: failed to sleep sandbox",
				"sandbox_id", apps[i].ID, "error", err)
			continue
		}
		slept++
	}
	if len(candidates) > 0 {
		logging.FromContext(ctx).InfoContext(ctx, "auto-sleep sweep",
			"candidates", len(candidates), "agents", len(agents), "apps", len(apps), "slept", slept)
	}
	return nil
}

// idleAgentSandboxes returns running agent sandboxes whose session is idle with
// neither session events nor preview-gateway traffic inside the threshold.
func (h *SandboxAutoSleepHandler) idleAgentSandboxes(ctx context.Context, cutoff time.Time) ([]model.Sandbox, error) {
	var sandboxes []model.Sandbox
	err := h.idleAgentSandboxesQuery(ctx, cutoff).
		Find(&sandboxes).Error
	return sandboxes, err
}

func (h *SandboxAutoSleepHandler) idleAgentSandboxesQuery(ctx context.Context, cutoff time.Time) *gorm.DB {
	return h.db.WithContext(ctx).
		Model(&model.Sandbox{}).
		Distinct("sandboxes.*").
		Joins("JOIN sessions ON sessions.sandbox_id = sandboxes.id").
		Joins("LEFT JOIN LATERAL (SELECT max(event_at) AS last_event FROM session_events se WHERE se.session_id = sessions.id) ev ON TRUE").
		Where("sessions.agent_turn_status = ?", model.SessionAgentTurnIdle).
		Where("sandboxes.status = ?", string(sandbox.StatusRunning)).
		Where("sandboxes.external_id <> ''").
		Where("GREATEST(COALESCE(ev.last_event, sessions.created_at), COALESCE(sandboxes.last_gateway_activity_at, sandboxes.created_at)) < ?", cutoff)
}

func (h *SandboxAutoSleepHandler) agentSandboxStillIdle(ctx context.Context, sandboxID uuid.UUID, cutoff time.Time) (bool, error) {
	var sb model.Sandbox
	err := h.idleAgentSandboxesQuery(ctx, cutoff).
		Where("sandboxes.id = ?", sandboxID).
		Take(&sb).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	return err == nil, err
}

// idleAppSandboxes returns running app sandboxes with no traffic for the
// threshold — the newer of the in-app ping and the mirrored gateway activity,
// or creation time if never hit. They wake on the next request via the alias.
func (h *SandboxAutoSleepHandler) idleAppSandboxes(ctx context.Context, cutoff time.Time) ([]model.Sandbox, error) {
	var sandboxes []model.Sandbox
	err := h.idleAppSandboxesQuery(ctx, cutoff).
		Find(&sandboxes).Error
	return sandboxes, err
}

func (h *SandboxAutoSleepHandler) idleAppSandboxesQuery(ctx context.Context, cutoff time.Time) *gorm.DB {
	return h.db.WithContext(ctx).
		Model(&model.Sandbox{}).
		Distinct("sandboxes.*").
		Joins("JOIN apps ON apps.sandbox_id = sandboxes.id AND apps.archived_at IS NULL").
		Where("sandboxes.status = ?", string(sandbox.StatusRunning)).
		Where("sandboxes.external_id <> ''").
		Where("COALESCE(GREATEST(sandboxes.last_app_activity_at, sandboxes.last_gateway_activity_at), sandboxes.created_at) < ?", cutoff)
}

func (h *SandboxAutoSleepHandler) appSandboxStillIdle(ctx context.Context, sandboxID uuid.UUID, cutoff time.Time) (bool, error) {
	var sb model.Sandbox
	err := h.idleAppSandboxesQuery(ctx, cutoff).
		Where("sandboxes.id = ?", sandboxID).
		Take(&sb).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	return err == nil, err
}
