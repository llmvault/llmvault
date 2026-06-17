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

const pluginInstallSyncTimeout = 15 * time.Minute

func init() {
	RegisterTaskBuilder(TypePluginInstallSync, func(payload []byte) (*asynq.Task, []asynq.Option, error) {
		var p PluginInstallSyncPayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return nil, nil, fmt.Errorf("unmarshal plugin install sync payload: %w", err)
		}
		return NewPluginInstallSyncTask(p)
	})
}

type PluginInstallSyncPayload struct {
	OrgID     uuid.UUID `json:"org_id"`
	PluginID  uuid.UUID `json:"plugin_id"`
	InstallID uuid.UUID `json:"install_id"`
}

func NewPluginInstallSyncTask(payload PluginInstallSyncPayload) (*asynq.Task, []asynq.Option, error) {
	if payload.OrgID == uuid.Nil {
		return nil, nil, fmt.Errorf("org_id is required")
	}
	if payload.PluginID == uuid.Nil {
		return nil, nil, fmt.Errorf("plugin_id is required")
	}
	if payload.InstallID == uuid.Nil {
		return nil, nil, fmt.Errorf("install_id is required")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal plugin install sync payload: %w", err)
	}
	opts := []asynq.Option{
		asynq.Queue(QueueDefault),
		asynq.MaxRetry(8),
		asynq.Timeout(pluginInstallSyncTimeout),
		asynq.Unique(10 * time.Minute),
	}
	return asynq.NewTask(TypePluginInstallSync, body), opts, nil
}

func EnqueuePluginInstallSync(ctx context.Context, enq enqueue.TaskEnqueuer, payload PluginInstallSyncPayload) error {
	if enq == nil {
		return nil
	}
	task, opts, err := NewPluginInstallSyncTask(payload)
	if err != nil {
		return err
	}
	if _, err := enq.EnqueueContext(ctx, task, opts...); err != nil {
		if errors.Is(err, asynq.ErrDuplicateTask) || errors.Is(err, asynq.ErrTaskIDConflict) {
			return nil
		}
		return fmt.Errorf("enqueue plugin install sync: %w", err)
	}
	return nil
}
