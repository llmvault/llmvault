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

const orgHivyAgentProvisionTimeout = 10 * time.Minute

type OrgHivyAgentSyncer interface {
	SyncOrgHivyAgent(ctx context.Context, orgID uuid.UUID) error
}

type OrgHivyAgentProvisionPayload struct {
	OrgID uuid.UUID `json:"org_id"`
}

func NewOrgHivyAgentProvisionTask(payload OrgHivyAgentProvisionPayload) (*asynq.Task, []asynq.Option, error) {
	if payload.OrgID == uuid.Nil {
		return nil, nil, fmt.Errorf("org_id is required")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal org Hivy agent provision payload: %w", err)
	}
	opts := []asynq.Option{
		asynq.Queue(QueueDefault),
		asynq.MaxRetry(8),
		asynq.Timeout(orgHivyAgentProvisionTimeout),
		asynq.Unique(10 * time.Minute),
	}
	return asynq.NewTask(TypeOrgHivyAgentProvision, body), opts, nil
}

func EnqueueOrgHivyAgentProvision(ctx context.Context, enq enqueue.TaskEnqueuer, orgID uuid.UUID) error {
	if enq == nil {
		return nil
	}
	task, opts, err := NewOrgHivyAgentProvisionTask(OrgHivyAgentProvisionPayload{OrgID: orgID})
	if err != nil {
		return err
	}
	if _, err := enq.EnqueueContext(ctx, task, opts...); err != nil {
		if errors.Is(err, asynq.ErrDuplicateTask) || errors.Is(err, asynq.ErrTaskIDConflict) {
			return nil
		}
		return fmt.Errorf("enqueue org Hivy agent provision: %w", err)
	}
	return nil
}

type OrgHivyAgentProvisionHandler struct {
	syncer OrgHivyAgentSyncer
}

func NewOrgHivyAgentProvisionHandler(syncer OrgHivyAgentSyncer) *OrgHivyAgentProvisionHandler {
	return &OrgHivyAgentProvisionHandler{syncer: syncer}
}

func (h *OrgHivyAgentProvisionHandler) Handle(ctx context.Context, task *asynq.Task) error {
	if h == nil || h.syncer == nil {
		return fmt.Errorf("org Hivy agent provision handler not configured")
	}
	var payload OrgHivyAgentProvisionPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal org Hivy agent provision payload: %w", err)
	}
	if payload.OrgID == uuid.Nil {
		return fmt.Errorf("org_id is required")
	}
	return h.syncer.SyncOrgHivyAgent(ctx, payload.OrgID)
}
