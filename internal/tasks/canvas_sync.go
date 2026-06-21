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
)

const canvasOrgSyncTimeout = 10 * time.Minute

type CanvasOrgSyncer interface {
	SyncCanvasOrg(ctx context.Context, orgID uuid.UUID) error
}

type CanvasOrgSyncPayload struct {
	OrgID uuid.UUID `json:"org_id"`
}

func NewCanvasOrgSyncTask(payload CanvasOrgSyncPayload) (*asynq.Task, []asynq.Option, error) {
	if payload.OrgID == uuid.Nil {
		return nil, nil, fmt.Errorf("org_id is required")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal canvas org sync payload: %w", err)
	}
	opts := []asynq.Option{
		asynq.Queue(QueueDefault),
		asynq.MaxRetry(8),
		asynq.Timeout(canvasOrgSyncTimeout),
		asynq.Unique(10 * time.Minute),
	}
	return asynq.NewTask(TypeCanvasOrgSync, body), opts, nil
}

func EnqueueCanvasOrgSync(ctx context.Context, enq enqueue.TaskEnqueuer, orgID uuid.UUID) error {
	if enq == nil {
		return nil
	}
	task, opts, err := NewCanvasOrgSyncTask(CanvasOrgSyncPayload{OrgID: orgID})
	if err != nil {
		return err
	}
	if _, err := enq.EnqueueContext(ctx, task, opts...); err != nil {
		if errors.Is(err, asynq.ErrDuplicateTask) || errors.Is(err, asynq.ErrTaskIDConflict) {
			return nil
		}
		return fmt.Errorf("enqueue canvas org sync: %w", err)
	}
	return nil
}

type CanvasOrgSyncHandler struct {
	syncer CanvasOrgSyncer
}

func NewCanvasOrgSyncHandler(syncer CanvasOrgSyncer) *CanvasOrgSyncHandler {
	return &CanvasOrgSyncHandler{syncer: syncer}
}

func (h *CanvasOrgSyncHandler) Handle(ctx context.Context, task *asynq.Task) error {
	if h == nil || h.syncer == nil {
		return nil
	}
	var payload CanvasOrgSyncPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal canvas org sync payload: %w", err)
	}
	if payload.OrgID == uuid.Nil {
		return fmt.Errorf("org_id is required")
	}
	return h.syncer.SyncCanvasOrg(ctx, payload.OrgID)
}
