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

	"github.com/usehivy/hivy/internal/cache"
	"github.com/usehivy/hivy/internal/memory"
	"github.com/usehivy/hivy/internal/model"
)

type MemoryEmbedPayload struct {
	MemoryID uuid.UUID `json:"memory_id"`
	Revision int       `json:"revision"`
}

type MemoryEmbeddingConfig struct {
	Model string
	Dim   uint32
}

func init() {
	RegisterTaskBuilder(TypeMemoryEmbed, func(payload []byte) (*asynq.Task, []asynq.Option, error) {
		var p MemoryEmbedPayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return nil, nil, fmt.Errorf("unmarshal memory embed payload: %w", err)
		}
		return NewMemoryEmbedTask(p)
	})
}

func NewMemoryEmbedTask(payload MemoryEmbedPayload) (*asynq.Task, []asynq.Option, error) {
	if payload.MemoryID == uuid.Nil {
		return nil, nil, fmt.Errorf("memory_id is required")
	}
	if payload.Revision <= 0 {
		return nil, nil, fmt.Errorf("revision is required")
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal memory embed payload: %w", err)
	}
	opts := []asynq.Option{
		asynq.Queue(QueueDefault),
		asynq.MaxRetry(5),
		asynq.Timeout(2 * time.Minute),
	}
	return asynq.NewTask(TypeMemoryEmbed, encoded), opts, nil
}

func EnqueueMemoryEmbed(ctx context.Context, enq enqueueTaskEnqueuer, memoryID uuid.UUID, revision int) error {
	if enq == nil {
		return nil
	}
	task, opts, err := NewMemoryEmbedTask(MemoryEmbedPayload{MemoryID: memoryID, Revision: revision})
	if err != nil {
		return err
	}
	_, err = enq.EnqueueContext(ctx, task, opts...)
	return err
}

type enqueueTaskEnqueuer interface {
	EnqueueContext(context.Context, *asynq.Task, ...asynq.Option) (*asynq.TaskInfo, error)
}

type MemoryEmbedHandler struct {
	db           *gorm.DB
	cacheManager *cache.Manager
	cfg          MemoryEmbeddingConfig
}

func NewMemoryEmbedHandler(db *gorm.DB, cacheManager *cache.Manager, cfg MemoryEmbeddingConfig) *MemoryEmbedHandler {
	return &MemoryEmbedHandler{db: db, cacheManager: cacheManager, cfg: cfg}
}

func (h *MemoryEmbedHandler) Handle(ctx context.Context, task *asynq.Task) error {
	var payload MemoryEmbedPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal payload: %w", err)
	}
	var row model.AgentMemory
	err := h.db.WithContext(ctx).
		Where("id = ? AND archived_at IS NULL", payload.MemoryID).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load memory: %w", err)
	}
	if row.EmbeddingRevision != payload.Revision {
		return nil
	}
	service := memory.NewService(memory.Config{
		DB:             h.db,
		CacheManager:   h.cacheManager,
		EmbeddingModel: h.embeddingModel(),
		EmbeddingDim:   h.embeddingDim(),
	})
	vector, err := service.EmbedMemoryContent(ctx, row.Content)
	if err != nil {
		_ = service.MarkEmbeddingFailed(ctx, row.ID, row.EmbeddingRevision, err)
		return err
	}
	if _, err := service.MarkEmbeddingReady(ctx, row.ID, row.EmbeddingRevision, vector); err != nil {
		return err
	}
	return nil
}

func (h *MemoryEmbedHandler) embeddingModel() string {
	if h != nil && h.cfg.Model != "" {
		return h.cfg.Model
	}
	return memory.DefaultEmbeddingModel
}

func (h *MemoryEmbedHandler) embeddingDim() uint32 {
	if h != nil && h.cfg.Dim > 0 {
		return h.cfg.Dim
	}
	return memory.DefaultEmbeddingDim
}

func memoryEmbeddingConfigFromDeps(deps *WorkerDeps) MemoryEmbeddingConfig {
	if deps == nil || deps.AgentCompile.Cfg == nil {
		return MemoryEmbeddingConfig{Model: memory.DefaultEmbeddingModel, Dim: memory.DefaultEmbeddingDim}
	}
	return MemoryEmbeddingConfig{
		Model: deps.AgentCompile.Cfg.MemoryEmbeddingModel,
		Dim:   deps.AgentCompile.Cfg.MemoryEmbeddingDim,
	}
}
