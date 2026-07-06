package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

// ChannelMemoriesDeletePayload identifies the channel whose memories should be
// hard-deleted after the channel itself is deleted.
type ChannelMemoriesDeletePayload struct {
	OrgID     uuid.UUID `json:"org_id"`
	ChannelID uuid.UUID `json:"channel_id"`
}

func init() {
	RegisterTaskBuilder(TypeChannelMemoriesDelete, func(payload []byte) (*asynq.Task, []asynq.Option, error) {
		var p ChannelMemoriesDeletePayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return nil, nil, fmt.Errorf("unmarshal channel memories delete payload: %w", err)
		}
		return NewChannelMemoriesDeleteTask(p)
	})
}

func NewChannelMemoriesDeleteTask(payload ChannelMemoriesDeletePayload) (*asynq.Task, []asynq.Option, error) {
	if payload.OrgID == uuid.Nil || payload.ChannelID == uuid.Nil {
		return nil, nil, fmt.Errorf("org_id and channel_id are required")
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal channel memories delete payload: %w", err)
	}
	opts := []asynq.Option{
		asynq.Queue(QueueDefault),
		asynq.MaxRetry(5),
		asynq.Timeout(5 * time.Minute),
	}
	return asynq.NewTask(TypeChannelMemoriesDelete, encoded), opts, nil
}

// EnqueueChannelMemoriesDelete schedules deletion of a deleted channel's
// memories. A nil enqueuer is a no-op so callers without async wiring degrade
// gracefully.
func EnqueueChannelMemoriesDelete(ctx context.Context, enq enqueueTaskEnqueuer, orgID, channelID uuid.UUID) error {
	if enq == nil {
		return nil
	}
	task, opts, err := NewChannelMemoriesDeleteTask(ChannelMemoriesDeletePayload{OrgID: orgID, ChannelID: channelID})
	if err != nil {
		return err
	}
	_, err = enq.EnqueueContext(ctx, task, opts...)
	return err
}

// ChannelMemoriesDeleteHandler hard-deletes every memory bound to a deleted
// channel. Memory rows carry their own embeddings, so a plain DELETE fully
// removes them with no external cleanup.
type ChannelMemoriesDeleteHandler struct {
	db *gorm.DB
}

func NewChannelMemoriesDeleteHandler(db *gorm.DB) *ChannelMemoriesDeleteHandler {
	return &ChannelMemoriesDeleteHandler{db: db}
}

func (h *ChannelMemoriesDeleteHandler) Handle(ctx context.Context, task *asynq.Task) error {
	var p ChannelMemoriesDeletePayload
	if err := json.Unmarshal(task.Payload(), &p); err != nil {
		return fmt.Errorf("unmarshal channel memories delete payload: %w", err)
	}
	res := h.db.WithContext(ctx).
		Where("org_id = ? AND channel_id = ?", p.OrgID, p.ChannelID).
		Delete(&model.AgentMemory{})
	if res.Error != nil {
		return fmt.Errorf("delete channel memories: %w", res.Error)
	}
	if res.RowsAffected > 0 {
		logging.FromContext(ctx).InfoContext(ctx, "deleted channel memories",
			"channel_id", p.ChannelID, "deleted", res.RowsAffected)
	}
	return nil
}
