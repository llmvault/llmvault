package handler

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	ragmodel "github.com/usehivy/hivy/internal/rag/model"
	ragtasks "github.com/usehivy/hivy/internal/rag/tasks"
)

func (h *OrgHandler) ensureWebsiteRAGSource(ctx context.Context, org *model.Org) {
	if org.Website == "" {
		return
	}
	if h.enq == nil {
		return
	}

	logger := logging.FromContext(ctx)
	var existing ragmodel.RAGSource
	err := h.db.WithContext(ctx).
		Where("org_id = ? AND kind = ? AND config->>'url' = ?", org.ID, ragmodel.RAGSourceKindWebsite, org.Website).
		First(&existing).Error
	if err == nil {
		if existing.LastSuccessfulIndexTime == nil && existing.TotalDocsIndexed == 0 {
			h.enqueueWebsiteRAGIngest(ctx, existing.ID)
		}
		return
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		logging.Capture(ctx, fmt.Errorf("ensure website rag source for org %s: lookup: %w", org.ID, err))
		return
	}

	src := &ragmodel.RAGSource{
		OrgIDValue: org.ID,
		KindValue:  ragmodel.RAGSourceKindWebsite,
		Name:       org.Website,
		Status:     ragmodel.RAGSourceStatusInitialIndexing,
		Enabled:    true,
		AccessType: ragmodel.AccessTypePublic,
		ConfigValue: model.JSON{
			"url":       org.Website,
			"max_pages": float64(100),
		},
		RefreshFreqSeconds: intPtr(86400),
	}

	if err := h.db.WithContext(ctx).Create(src).Error; err != nil {
		logging.Capture(ctx, fmt.Errorf("ensure website rag source for org %s: create: %w", org.ID, err))
		return
	}
	logger.InfoContext(ctx, "created website rag source", "org_id", org.ID, "rag_source_id", src.ID, "website", org.Website)
	h.enqueueWebsiteRAGIngest(ctx, src.ID)
}

func (h *OrgHandler) enqueueWebsiteRAGIngest(ctx context.Context, ragSourceID uuid.UUID) {
	task, err := ragtasks.NewIngestTask(ragtasks.IngestPayload{RAGSourceID: ragSourceID})
	if err != nil {
		logging.Capture(ctx, fmt.Errorf("ensure website rag: build ingest task for %s: %w", ragSourceID, err))
		return
	}
	if _, err := h.enq.Enqueue(task, asynq.Unique(60*time.Second)); err != nil {
		if errors.Is(err, asynq.ErrDuplicateTask) {
			return
		}
		logging.Capture(ctx, fmt.Errorf("ensure website rag: enqueue ingest for %s: %w", ragSourceID, err))
	}
}
