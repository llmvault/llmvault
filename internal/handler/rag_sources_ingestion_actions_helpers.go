package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/logging"
	ragmodel "github.com/usehivy/hivy/internal/rag/model"
	ragtasks "github.com/usehivy/hivy/internal/rag/tasks"
)

type sourceIngestSnapshot struct {
	Status             ragmodel.RAGSourceStatus
	Enabled            bool
	RepeatedErrorState bool
}

type reservedIngest struct {
	SourceID uuid.UUID
	Attempt  *ragmodel.RAGIndexAttempt
	Existing *ragmodel.RAGIndexAttempt
	Snapshot sourceIngestSnapshot
}

func (h *RAGSourceHandler) reserveIngest(
	ctx context.Context,
	orgID, sourceID uuid.UUID,
	action ingestAction,
) (*reservedIngest, error) {
	result := &reservedIngest{SourceID: sourceID}
	err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var src ragmodel.RAGSource
		if err := tx.WithContext(ctx).
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND org_id = ?", sourceID, orgID).
			First(&src).Error; err != nil {
			return fmt.Errorf("lock rag source for %s: %w", action, err)
		}
		if src.Status == ragmodel.RAGSourceStatusDeleting {
			return errRAGIngestAlreadyRunning
		}

		result.Snapshot = sourceIngestSnapshot{
			Status:             src.Status,
			Enabled:            src.Enabled,
			RepeatedErrorState: src.InRepeatedErrorState,
		}

		var active ragmodel.RAGIndexAttempt
		activeErr := tx.WithContext(ctx).
			Where("rag_source_id = ? AND org_id = ? AND status IN ?", sourceID, orgID, []ragmodel.IndexingStatus{
				ragmodel.IndexingStatusNotStarted,
				ragmodel.IndexingStatusInProgress,
			}).
			Order("time_created DESC, id DESC").
			First(&active).Error
		if activeErr != nil && !errors.Is(activeErr, gorm.ErrRecordNotFound) {
			return fmt.Errorf("load active rag ingest attempt: %w", activeErr)
		}

		updates := map[string]any{"enabled": true}
		switch action {
		case ingestActionResume:
			if src.Status != ragmodel.RAGSourceStatusPaused {
				return errRAGSourceNotPaused
			}
			updates["status"] = ragmodel.RAGSourceStatusActive
		case ingestActionRetry:
			if activeErr == nil {
				return errRAGIngestAlreadyRunning
			}
			latest, err := latestIngestAttempt(ctx, tx, orgID, sourceID)
			if err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return errRAGLatestAttemptNotFailed
				}
				return err
			}
			if latest.Status != ragmodel.IndexingStatusFailed {
				return errRAGLatestAttemptNotFailed
			}
			if src.LastSuccessfulIndexTime == nil {
				updates["status"] = ragmodel.RAGSourceStatusInitialIndexing
			} else {
				updates["status"] = ragmodel.RAGSourceStatusActive
			}
			updates["in_repeated_error_state"] = false
		default:
			return fmt.Errorf("unsupported rag ingest action %q", action)
		}

		res := tx.WithContext(ctx).
			Model(&ragmodel.RAGSource{}).
			Where("id = ? AND org_id = ?", sourceID, orgID).
			Updates(updates)
		if res.Error != nil {
			return fmt.Errorf("update rag source for %s: %w", action, res.Error)
		}
		if res.RowsAffected != 1 {
			return fmt.Errorf("update rag source for %s: %w", action, gorm.ErrRecordNotFound)
		}

		if activeErr == nil {
			result.Existing = &active
			return nil
		}

		now := time.Now()
		attempt := &ragmodel.RAGIndexAttempt{
			OrgID:            orgID,
			RAGSourceID:      sourceID,
			Status:           ragmodel.IndexingStatusNotStarted,
			LastProgressTime: &now,
			TimeCreated:      now,
			TimeUpdated:      now,
		}
		var checkpointed ragmodel.RAGIndexAttempt
		checkpointErr := tx.WithContext(ctx).
			Select("checkpoint_pointer").
			Where("rag_source_id = ? AND org_id = ? AND checkpoint_pointer IS NOT NULL", sourceID, orgID).
			Order("time_created DESC, id DESC").
			First(&checkpointed).Error
		if checkpointErr == nil {
			attempt.CheckpointPointer = checkpointed.CheckpointPointer
		} else if !errors.Is(checkpointErr, gorm.ErrRecordNotFound) {
			return fmt.Errorf("load rag ingest checkpoint: %w", checkpointErr)
		}
		if err := tx.WithContext(ctx).Create(attempt).Error; err != nil {
			return fmt.Errorf("reserve rag ingest attempt: %w", err)
		}
		result.Attempt = attempt
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (h *RAGSourceHandler) latestIngestAttempt(
	ctx context.Context,
	orgID, sourceID uuid.UUID,
) (*ragmodel.RAGIndexAttempt, error) {
	return latestIngestAttempt(ctx, h.db, orgID, sourceID)
}

func latestIngestAttempt(
	ctx context.Context,
	db *gorm.DB,
	orgID, sourceID uuid.UUID,
) (*ragmodel.RAGIndexAttempt, error) {
	var attempt ragmodel.RAGIndexAttempt
	if err := db.WithContext(ctx).
		Where("rag_source_id = ? AND org_id = ?", sourceID, orgID).
		Order("time_created DESC, id DESC").
		First(&attempt).Error; err != nil {
		return nil, err
	}
	return &attempt, nil
}

func (h *RAGSourceHandler) enqueueReservedIngest(ctx context.Context, attempt *ragmodel.RAGIndexAttempt) error {
	if attempt == nil {
		return errors.New("missing reserved rag ingest attempt")
	}
	task, err := ragtasks.NewIngestTask(ragtasks.IngestPayload{
		RAGSourceID: attempt.RAGSourceID,
		AttemptID:   &attempt.ID,
	})
	if err != nil {
		return err
	}
	opts := append(ragtasks.IngestEnqueueOptions(attempt.RAGSourceID), asynq.Unique(uniqueTriggerTTL))
	_, err = h.enq.EnqueueContext(ctx, task, opts...)
	return err
}

func (h *RAGSourceHandler) rollbackReservedIngest(ctx context.Context, orgID uuid.UUID, reserved *reservedIngest) {
	if reserved == nil || reserved.Attempt == nil {
		return
	}
	err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		deleted := tx.WithContext(ctx).
			Where("id = ? AND rag_source_id = ? AND org_id = ? AND status = ?",
				reserved.Attempt.ID, reserved.SourceID, orgID, ragmodel.IndexingStatusNotStarted).
			Delete(&ragmodel.RAGIndexAttempt{})
		if deleted.Error != nil {
			return deleted.Error
		}
		if deleted.RowsAffected != 1 {
			return errors.New("reserved rag ingest attempt was already claimed")
		}
		updated := tx.WithContext(ctx).
			Model(&ragmodel.RAGSource{}).
			Where("id = ? AND org_id = ?", reserved.SourceID, orgID).
			Updates(map[string]any{
				"status":                  reserved.Snapshot.Status,
				"enabled":                 reserved.Snapshot.Enabled,
				"in_repeated_error_state": reserved.Snapshot.RepeatedErrorState,
			})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
	if err != nil {
		logging.Capture(ctx, fmt.Errorf("rollback reserved rag ingest source=%s attempt=%s: %w", reserved.SourceID, reserved.Attempt.ID, err))
	}
}

func writeRAGIngestActionError(ctx context.Context, w http.ResponseWriter, sourceID uuid.UUID, err error) {
	switch {
	case errors.Is(err, errRAGSourceNotPaused):
		writeJSON(w, http.StatusConflict, errorResponse{Error: "source is not paused"})
	case errors.Is(err, errRAGLatestAttemptNotFailed):
		writeJSON(w, http.StatusConflict, errorResponse{Error: "latest ingest attempt did not fail"})
	case errors.Is(err, errRAGIngestAlreadyRunning):
		writeJSON(w, http.StatusConflict, errorResponse{Error: "ingestion is already queued or running"})
	case errors.Is(err, gorm.ErrRecordNotFound):
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "source not found"})
	default:
		logging.FromContext(ctx).ErrorContext(ctx, "prepare rag ingest action", "source_id", sourceID, "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to prepare ingest task"})
	}
}
