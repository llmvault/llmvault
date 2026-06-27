package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"github.com/usehivy/hivy/internal/enqueue"
)

type SlackAppMentionPayload struct {
	SlackThreadEventID uuid.UUID `json:"slack_thread_event_id"`
}

func init() {
	RegisterTaskBuilder(TypeSlackAppMention, func(payload []byte) (*asynq.Task, []asynq.Option, error) {
		var p SlackAppMentionPayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return nil, nil, fmt.Errorf("unmarshal slack app mention payload: %w", err)
		}
		return NewSlackAppMentionTask(p, "")
	})
}

func NewSlackAppMentionTask(payload SlackAppMentionPayload, deliveryID string) (*asynq.Task, []asynq.Option, error) {
	if payload.SlackThreadEventID == uuid.Nil {
		return nil, nil, fmt.Errorf("slack_thread_event_id is required")
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal slack app mention payload: %w", err)
	}
	taskID := "slack-app-mention:" + payload.SlackThreadEventID.String()
	if strings.TrimSpace(deliveryID) != "" {
		taskID = "slack-app-mention:" + strings.TrimSpace(deliveryID)
	}
	opts := []asynq.Option{
		asynq.Queue(QueueCritical),
		asynq.MaxRetry(5),
		asynq.Timeout(12 * time.Minute),
		asynq.TaskID(taskID),
	}
	return asynq.NewTask(TypeSlackAppMention, encoded), opts, nil
}

func EnqueueSlackAppMention(ctx context.Context, enq enqueue.TaskEnqueuer, eventID uuid.UUID, deliveryID string) error {
	if enq == nil {
		return fmt.Errorf("task enqueuer is required")
	}
	task, opts, err := NewSlackAppMentionTask(SlackAppMentionPayload{SlackThreadEventID: eventID}, deliveryID)
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

func isAsynqDuplicateTask(err error) bool {
	return errors.Is(err, asynq.ErrTaskIDConflict) || errors.Is(err, asynq.ErrDuplicateTask)
}
