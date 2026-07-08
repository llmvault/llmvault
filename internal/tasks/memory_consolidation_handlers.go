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
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/memory"
	"github.com/usehivy/hivy/internal/model"
)

type MemoryConsolidationSweepHandler struct {
	db       *gorm.DB
	enqueuer enqueue.TaskEnqueuer
}

func NewMemoryConsolidationSweepHandler(db *gorm.DB, enqueuer enqueue.TaskEnqueuer) *MemoryConsolidationSweepHandler {
	return &MemoryConsolidationSweepHandler{db: db, enqueuer: enqueuer}
}

func (h *MemoryConsolidationSweepHandler) Handle(ctx context.Context, _ *asynq.Task) error {
	if h == nil || h.db == nil || h.enqueuer == nil {
		return nil
	}
	svc := memory.NewService(memory.Config{DB: h.db})
	channels, err := svc.ChannelsWithUnconsolidatedFacts(ctx, memoryConsolidationSweepLimit)
	if err != nil {
		return err
	}
	for _, ch := range channels {
		if err := EnqueueMemoryConsolidate(ctx, h.enqueuer, ch.OrgID, ch.ChannelID); err != nil {
			return fmt.Errorf("enqueue memory consolidation: %w", err)
		}
	}
	return nil
}

// MemoryObservationExpireHandler is the nightly lifecycle job: archive
// observations whose expires_at has passed, then refresh every affected
// channel memory digest.
type MemoryObservationExpireHandler struct {
	db           *gorm.DB
	cacheManager *cache.Manager
	memoryCfg    MemoryEmbeddingConfig
	now          func() time.Time
}

func NewMemoryObservationExpireHandler(db *gorm.DB, cacheManager *cache.Manager, memoryCfg MemoryEmbeddingConfig) *MemoryObservationExpireHandler {
	return &MemoryObservationExpireHandler{db: db, cacheManager: cacheManager, memoryCfg: memoryCfg}
}

func (h *MemoryObservationExpireHandler) Handle(ctx context.Context, _ *asynq.Task) error {
	if h == nil || h.db == nil {
		return nil
	}
	now := time.Now().UTC()
	if h.now != nil {
		now = h.now().UTC()
	}
	svc := memory.NewService(memory.Config{
		DB:             h.db,
		CacheManager:   h.cacheManager,
		EmbeddingModel: h.memoryCfg.Model,
		EmbeddingDim:   h.memoryCfg.Dim,
	})
	expired, err := svc.ExpireObservations(ctx, now)
	if err != nil {
		return err
	}
	if len(expired) == 0 {
		return nil
	}
	affected := map[uuid.UUID]uuid.UUID{} // channel -> org
	for _, obs := range expired {
		if obs.ChannelID != nil {
			affected[*obs.ChannelID] = obs.OrgID
			continue
		}
		// Org-wide expiry touches every channel digest that folds org
		// observations in; refresh all digests recorded for the org.
		var digests []model.ChannelMemoryDigest
		if err := h.db.WithContext(ctx).
			Where("org_id = ?", obs.OrgID).
			Find(&digests).Error; err != nil {
			return fmt.Errorf("list org digests for expiry: %w", err)
		}
		for _, digest := range digests {
			affected[digest.ChannelID] = digest.OrgID
		}
	}
	logger := logging.FromContext(ctx)
	for channelID, orgID := range affected {
		if err := svc.RecomputeChannelDigest(ctx, orgID, channelID); err != nil {
			logger.WarnContext(ctx, "digest recompute after expiry failed",
				"org_id", orgID, "channel_id", channelID, "error", err)
		}
	}
	logger.InfoContext(ctx, "expired observations archived",
		"count", len(expired), "digests_refreshed", len(affected))
	return nil
}

// ObservationEmbedHandler retries observation embeddings that failed during
// the consolidation worker's synchronous pass.
type ObservationEmbedHandler struct {
	db           *gorm.DB
	cacheManager *cache.Manager
	cfg          MemoryEmbeddingConfig
}

func NewObservationEmbedHandler(db *gorm.DB, cacheManager *cache.Manager, cfg MemoryEmbeddingConfig) *ObservationEmbedHandler {
	return &ObservationEmbedHandler{db: db, cacheManager: cacheManager, cfg: cfg}
}

func (h *ObservationEmbedHandler) Handle(ctx context.Context, task *asynq.Task) error {
	var payload ObservationEmbedPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal observation embed payload: %w", err)
	}
	svc := memory.NewService(memory.Config{
		DB:             h.db,
		CacheManager:   h.cacheManager,
		EmbeddingModel: h.cfg.Model,
		EmbeddingDim:   h.cfg.Dim,
	})
	obs, err := svc.LoadObservation(ctx, payload.ObservationID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if obs.ArchivedAt != nil || obs.EmbeddingRevision != payload.Revision ||
		obs.EmbeddingStatus == model.AgentMemoryEmbeddingReady {
		return nil
	}
	vectors, err := svc.EmbedContents(ctx, obs.OrgID, []string{obs.Content})
	if err != nil {
		_ = svc.MarkObservationEmbeddingFailed(ctx, obs.ID, payload.Revision, err)
		return err
	}
	if _, err := svc.MarkObservationEmbeddingReady(ctx, obs.ID, payload.Revision, vectors[0]); err != nil {
		return err
	}
	return nil
}
