package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"github.com/usehivy/hivy/internal/enqueue"
)

type SlackReactionTriggerPayload struct {
	SlackThreadEventID uuid.UUID `json:"slack_thread_event_id"`
}

func init() {
	RegisterTaskBuilder(TypeSlackReactionTrigger, func(payload []byte) (*asynq.Task, []asynq.Option, error) {
		var p SlackReactionTriggerPayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return nil, nil, fmt.Errorf("unmarshal slack reaction trigger payload: %w", err)
		}
		return NewSlackReactionTriggerTask(p, "")
	})
}

func NewSlackReactionTriggerTask(payload SlackReactionTriggerPayload, deliveryID string) (*asynq.Task, []asynq.Option, error) {
	if payload.SlackThreadEventID == uuid.Nil {
		return nil, nil, fmt.Errorf("slack_thread_event_id is required")
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal slack reaction trigger payload: %w", err)
	}
	taskID := "slack-reaction-trigger:" + payload.SlackThreadEventID.String()
	if strings.TrimSpace(deliveryID) != "" {
		taskID = "slack-reaction-trigger:" + strings.TrimSpace(deliveryID)
	}
	opts := []asynq.Option{
		asynq.Queue(QueueCritical),
		asynq.MaxRetry(5),
		asynq.Timeout(12 * time.Minute),
		asynq.TaskID(taskID),
	}
	return asynq.NewTask(TypeSlackReactionTrigger, encoded), opts, nil
}

func EnqueueSlackReactionTrigger(ctx context.Context, enq enqueue.TaskEnqueuer, eventID uuid.UUID, deliveryID string) error {
	if enq == nil {
		return fmt.Errorf("task enqueuer is required")
	}
	task, opts, err := NewSlackReactionTriggerTask(SlackReactionTriggerPayload{SlackThreadEventID: eventID}, deliveryID)
	if err != nil {
		return err
	}
	if _, err := enq.EnqueueContext(ctx, task, opts...); err != nil {
		if isAsynqDuplicateTask(err) {
			return nil
		}
		return err
	}
	return nil
}
