package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/sandbox"
)

const (
	sandboxWarmPoolReconcileTimeout = 10 * time.Minute
	sandboxWarmSlotCheckTimeout     = 30 * time.Second
	sandboxWarmSlotCheckDelay       = 15 * time.Second
	sandboxWarmSlotReadyTimeout     = 5 * time.Minute
)

type SandboxWarmPoolReconcilePayload struct {
	ProviderID string `json:"provider_id"`
	sandbox.WarmPoolProfile
}

type SandboxWarmSlotCheckPayload struct {
	ProviderID string    `json:"provider_id"`
	SlotID     uuid.UUID `json:"slot_id"`
	StartedAt  time.Time `json:"started_at"`
}

func NewSandboxWarmPoolReconcileTask(payload SandboxWarmPoolReconcilePayload) (*asynq.Task, []asynq.Option, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, err
	}
	opts := []asynq.Option{
		asynq.Queue(QueueDefault),
		asynq.MaxRetry(3),
		asynq.Timeout(sandboxWarmPoolReconcileTimeout),
		// The unique window must cover the full task timeout, or a second reconcile
		// enqueues while the first is still provisioning. (The in-Reconcile advisory
		// lock is the hard serialisation; this just curbs duplicates.)
		asynq.Unique(sandboxWarmPoolReconcileTimeout),
	}
	return asynq.NewTask(TypeSandboxWarmPoolReconcile, body), opts, nil
}

func NewSandboxWarmSlotCheckTask(payload SandboxWarmSlotCheckPayload) (*asynq.Task, []asynq.Option, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, err
	}
	opts := []asynq.Option{
		asynq.Queue(QueueDefault),
		asynq.MaxRetry(5),
		asynq.Timeout(sandboxWarmSlotCheckTimeout),
	}
	return asynq.NewTask(TypeSandboxWarmSlotCheck, body), opts, nil
}

func EnqueueSandboxWarmPoolReconcile(ctx context.Context, enqueuer enqueue.TaskEnqueuer, providerID string, profile sandbox.WarmPoolProfile) error {
	if enqueuer == nil {
		return nil
	}
	task, opts, err := NewSandboxWarmPoolReconcileTask(SandboxWarmPoolReconcilePayload{
		ProviderID:      providerID,
		WarmPoolProfile: profile,
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

func EnqueueSandboxWarmSlotCheck(ctx context.Context, enqueuer enqueue.TaskEnqueuer, providerID string, slotID uuid.UUID, startedAt time.Time, delay time.Duration) error {
	if enqueuer == nil {
		return nil
	}
	if startedAt.IsZero() {
		startedAt = time.Now()
	}
	task, opts, err := NewSandboxWarmSlotCheckTask(SandboxWarmSlotCheckPayload{
		ProviderID: providerID,
		SlotID:     slotID,
		StartedAt:  startedAt,
	})
	if err != nil {
		return err
	}
	if delay > 0 {
		opts = append(opts, asynq.ProcessIn(delay))
	}
	opts = append(opts, asynq.TaskID(fmt.Sprintf("sandbox-warm-slot-check:%s:%d", slotID, time.Now().UnixNano())))
	if _, err := enqueuer.EnqueueContext(ctx, task, opts...); err != nil {
		if errors.Is(err, asynq.ErrDuplicateTask) || errors.Is(err, asynq.ErrTaskIDConflict) {
			return nil
		}
		return err
	}
	return nil
}

func EnqueueConfiguredWarmPoolReconciles(ctx context.Context, enqueuer enqueue.TaskEnqueuer, orchestrator *sandbox.Orchestrator) {
	if orchestrator == nil || orchestrator.WarmPool() == nil {
		return
	}
	for _, profile := range sandbox.ConfiguredWarmPoolProfiles(orchestrator.Config()) {
		_ = EnqueueSandboxWarmPoolReconcile(ctx, enqueuer, orchestrator.ProviderID(), profile)
	}
}

type SandboxWarmPoolReconcileHandler struct {
	orchestrator *sandbox.Orchestrator
	enqueuer     enqueue.TaskEnqueuer
}

func NewSandboxWarmPoolReconcileHandler(orchestrator *sandbox.Orchestrator, enqueuer enqueue.TaskEnqueuer) *SandboxWarmPoolReconcileHandler {
	return &SandboxWarmPoolReconcileHandler{orchestrator: orchestrator, enqueuer: enqueuer}
}

func (h *SandboxWarmPoolReconcileHandler) Handle(ctx context.Context, task *asynq.Task) error {
	if h.orchestrator == nil || h.orchestrator.WarmPool() == nil {
		return nil
	}
	var payload SandboxWarmPoolReconcilePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return err
	}
	if payload.ProviderID != h.orchestrator.ProviderID() {
		return nil
	}
	_, err := h.orchestrator.WarmPool().Reconcile(ctx, payload.WarmPoolProfile, func(ctx context.Context, slotID uuid.UUID) error {
		if err := EnqueueSandboxWarmSlotCheck(ctx, h.enqueuer, payload.ProviderID, slotID, time.Now(), sandboxWarmSlotCheckDelay); err != nil {
			return fmt.Errorf("enqueue warm slot check: %w", err)
		}
		return nil
	})
	return err
}

type SandboxWarmSlotCheckHandler struct {
	orchestrator *sandbox.Orchestrator
	enqueuer     enqueue.TaskEnqueuer
}

func NewSandboxWarmSlotCheckHandler(orchestrator *sandbox.Orchestrator, enqueuer enqueue.TaskEnqueuer) *SandboxWarmSlotCheckHandler {
	return &SandboxWarmSlotCheckHandler{orchestrator: orchestrator, enqueuer: enqueuer}
}

func (h *SandboxWarmSlotCheckHandler) Handle(ctx context.Context, task *asynq.Task) error {
	if h.orchestrator == nil || h.orchestrator.WarmPool() == nil {
		return nil
	}
	var payload SandboxWarmSlotCheckPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return err
	}
	if payload.ProviderID != h.orchestrator.ProviderID() {
		return nil
	}
	result, err := h.orchestrator.WarmPool().CheckWarmSlot(ctx, payload.SlotID)
	if err != nil {
		return err
	}
	if result == nil || !result.Pending {
		return nil
	}
	if payload.StartedAt.IsZero() {
		payload.StartedAt = time.Now()
	}
	if time.Since(payload.StartedAt) >= sandboxWarmSlotReadyTimeout {
		_ = h.orchestrator.WarmPool().MarkError(ctx, payload.SlotID,
			fmt.Sprintf("warm slot did not become ready after %s", sandboxWarmSlotReadyTimeout))
		if profile, profileErr := h.orchestrator.WarmPool().SlotProfile(ctx, payload.SlotID); profileErr == nil {
			_ = EnqueueSandboxWarmPoolReconcile(ctx, h.enqueuer, payload.ProviderID, profile)
		}
		return fmt.Errorf("warm slot %s did not become ready after %s", payload.SlotID, sandboxWarmSlotReadyTimeout)
	}
	return EnqueueSandboxWarmSlotCheck(ctx, h.enqueuer, payload.ProviderID, payload.SlotID, payload.StartedAt, sandboxWarmSlotCheckDelay)
}
