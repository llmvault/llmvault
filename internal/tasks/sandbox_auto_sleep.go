package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sandbox"
)

const defaultAutoSleepIdleThreshold = 15 * time.Second

const (
	sandboxSleepKindAgent = "agent"
	sandboxSleepKindApp   = "app"
)

// SandboxSleepPayload identifies one independently sleeping sandbox. Kind
// selects the activity predicate used by the final pre-stop recheck.
type SandboxSleepPayload struct {
	SandboxID uuid.UUID `json:"sandbox_id"`
	Kind      string    `json:"kind"`
}

type sandboxSleepCandidate struct {
	Sandbox model.Sandbox
	Kind    string
}

// NewSandboxSleepTask builds one idempotent lifecycle task per discovered
// sandbox. The dedicated queue is intentionally configured for 100-way
// concurrency by the worker process.
func NewSandboxSleepTask(payload SandboxSleepPayload) (*asynq.Task, []asynq.Option, error) {
	if payload.SandboxID == uuid.Nil {
		return nil, nil, fmt.Errorf("sandbox_id is required")
	}
	if payload.Kind != sandboxSleepKindAgent && payload.Kind != sandboxSleepKindApp {
		return nil, nil, fmt.Errorf("invalid sandbox sleep kind %q", payload.Kind)
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal sandbox sleep payload: %w", err)
	}
	return asynq.NewTask(TypeSandboxSleep, encoded), []asynq.Option{
		asynq.Queue(QueueSandboxLifecycle),
		asynq.MaxRetry(3),
		asynq.Timeout(45 * time.Second),
		// Periodic scans can rediscover a sandbox while its stop is in flight.
		asynq.Unique(30 * time.Second),
	}, nil
}

func enqueueSandboxSleep(ctx context.Context, enqueuer enqueue.TaskEnqueuer, candidate sandboxSleepCandidate) error {
	if enqueuer == nil {
		return nil
	}
	task, opts, err := NewSandboxSleepTask(SandboxSleepPayload{
		SandboxID: candidate.Sandbox.ID,
		Kind:      candidate.Kind,
	})
	if err != nil {
		return err
	}
	if _, err := enqueuer.EnqueueContext(ctx, task, opts...); err != nil {
		if errors.Is(err, asynq.ErrDuplicateTask) || errors.Is(err, asynq.ErrTaskIDConflict) {
			return nil
		}
		return err
	}
	return nil
}

// SandboxAutoSleepHandler is a fast scanner. It performs no runner network
// calls: every discovered sandbox becomes its own sandbox:sleep task so a
// large idle cohort can stop concurrently.
type SandboxAutoSleepHandler struct {
	db          *gorm.DB
	enqueuer    enqueue.TaskEnqueuer
	idleTimeout time.Duration
}

func NewSandboxAutoSleepHandler(db *gorm.DB, enqueuer enqueue.TaskEnqueuer, idleTimeout time.Duration) *SandboxAutoSleepHandler {
	if idleTimeout <= 0 {
		idleTimeout = defaultAutoSleepIdleThreshold
	}
	return &SandboxAutoSleepHandler{db: db, enqueuer: enqueuer, idleTimeout: idleTimeout}
}

func (h *SandboxAutoSleepHandler) Handle(ctx context.Context, _ *asynq.Task) error {
	cutoff := time.Now().Add(-h.idleTimeout)
	agents, err := h.idleAgentSandboxes(ctx, cutoff)
	if err != nil {
		return err
	}
	apps, err := h.idleAppSandboxes(ctx, cutoff)
	if err != nil {
		return err
	}
	candidates := make([]sandboxSleepCandidate, 0, len(agents)+len(apps))
	for _, sb := range agents {
		candidates = append(candidates, sandboxSleepCandidate{Sandbox: sb, Kind: sandboxSleepKindAgent})
	}
	for _, sb := range apps {
		candidates = append(candidates, sandboxSleepCandidate{Sandbox: sb, Kind: sandboxSleepKindApp})
	}

	enqueued, enqueueErr := h.enqueueCandidates(ctx, candidates)
	if len(candidates) > 0 {
		logging.FromContext(ctx).InfoContext(ctx, "auto-sleep sweep dispatched",
			"candidates", len(candidates), "agents", len(agents), "apps", len(apps), "enqueued", enqueued)
	}
	return enqueueErr
}

func (h *SandboxAutoSleepHandler) enqueueCandidates(ctx context.Context, candidates []sandboxSleepCandidate) (int, error) {
	enqueued := 0
	errs := make([]error, 0)
	for _, candidate := range candidates {
		if err := enqueueSandboxSleep(ctx, h.enqueuer, candidate); err != nil {
			errs = append(errs, fmt.Errorf("enqueue sandbox %s sleep: %w", candidate.Sandbox.ID, err))
			continue
		}
		enqueued++
	}
	return enqueued, errors.Join(errs...)
}

// SandboxSleepHandler owns the slow network operation for exactly one
// sandbox. It rechecks activity immediately before stopping, making delayed or
// retried jobs safe when a user has resumed work.
type SandboxSleepHandler struct {
	db           *gorm.DB
	orchestrator *sandbox.Orchestrator
	idleTimeout  time.Duration
}

func NewSandboxSleepHandler(db *gorm.DB, orchestrator *sandbox.Orchestrator, idleTimeout time.Duration) *SandboxSleepHandler {
	if idleTimeout <= 0 {
		idleTimeout = defaultAutoSleepIdleThreshold
	}
	return &SandboxSleepHandler{db: db, orchestrator: orchestrator, idleTimeout: idleTimeout}
}

func (h *SandboxSleepHandler) Handle(ctx context.Context, task *asynq.Task) error {
	var payload SandboxSleepPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal sandbox sleep payload: %w", err)
	}
	if payload.SandboxID == uuid.Nil {
		return fmt.Errorf("sandbox_id is required")
	}
	if payload.Kind != sandboxSleepKindAgent && payload.Kind != sandboxSleepKindApp {
		return fmt.Errorf("invalid sandbox sleep kind %q", payload.Kind)
	}

	cutoff := time.Now().Add(-h.idleTimeout)
	checker := &SandboxAutoSleepHandler{db: h.db, idleTimeout: h.idleTimeout}
	var idle bool
	var err error
	switch payload.Kind {
	case sandboxSleepKindAgent:
		idle, err = checker.agentSandboxStillIdle(ctx, payload.SandboxID, cutoff)
	case sandboxSleepKindApp:
		idle, err = checker.appSandboxStillIdle(ctx, payload.SandboxID, cutoff)
	}
	if err != nil {
		return fmt.Errorf("recheck sandbox %s idle state: %w", payload.SandboxID, err)
	}
	if !idle {
		return nil
	}

	var sb model.Sandbox
	if err := h.db.WithContext(ctx).First(&sb, "id = ?", payload.SandboxID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return fmt.Errorf("load sandbox %s: %w", payload.SandboxID, err)
	}
	if err := h.orchestrator.SleepSandbox(ctx, &sb); err != nil {
		return fmt.Errorf("sleep sandbox %s: %w", payload.SandboxID, err)
	}
	logging.FromContext(ctx).InfoContext(ctx, "auto-sleep sandbox complete", "sandbox_id", payload.SandboxID, "kind", payload.Kind)
	return nil
}

// idleAgentSandboxes returns running agent sandboxes whose session is idle with
// neither session events nor preview-gateway traffic inside the configured timeout.
func (h *SandboxAutoSleepHandler) idleAgentSandboxes(ctx context.Context, cutoff time.Time) ([]model.Sandbox, error) {
	var sandboxes []model.Sandbox
	err := h.idleAgentSandboxesQuery(ctx, cutoff).Find(&sandboxes).Error
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
	err := h.idleAgentSandboxesQuery(ctx, cutoff).Where("sandboxes.id = ?", sandboxID).Take(&sb).Error
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
	err := h.idleAppSandboxesQuery(ctx, cutoff).Find(&sandboxes).Error
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
	err := h.idleAppSandboxesQuery(ctx, cutoff).Where("sandboxes.id = ?", sandboxID).Take(&sb).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	return err == nil, err
}
