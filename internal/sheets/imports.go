package sheets

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

// Import job statuses.
const (
	ImportStatusPending   = "pending"
	ImportStatusRunning   = "running"
	ImportStatusCompleted = "completed"
	ImportStatusFailed    = "failed"
)

// ImportEnqueuer schedules the CSV import worker for a persisted job. Phase 4
// plugs in the Asynq-backed implementation; when unset, jobs stay pending.
type ImportEnqueuer interface {
	EnqueueSheetCSVImport(ctx context.Context, jobID uuid.UUID) error
}

type CreateImportJobRequest struct {
	ObjectKey string
	Options   model.JSON
}

// CreateImportJob persists a CSV import job for a page and hands it to the
// enqueuer. The object key must belong to the caller's org.
func (s *Service) CreateImportJob(ctx context.Context, orgID, pageID uuid.UUID, req CreateImportJobRequest, actor Actor) (*model.SheetImportJob, error) {
	page, err := s.loadPage(ctx, orgID, pageID)
	if err != nil {
		return nil, err
	}
	key := strings.TrimSpace(req.ObjectKey)
	prefix := OrgAttachmentPrefix(orgID)
	if !strings.HasPrefix(key, prefix) || len(key) <= len(prefix) {
		return nil, &AttachmentError{Key: key, Message: "object key is not owned by this org"}
	}
	options := req.Options
	if options == nil {
		options = model.JSON{}
	}
	job := model.SheetImportJob{
		OrgID:            orgID,
		PageID:           page.ID,
		ObjectKey:        key,
		Status:           ImportStatusPending,
		Options:          options,
		CreatedByAgentID: actor.AgentID,
		CreatedByUserID:  actor.UserID,
	}
	if err := s.db.WithContext(ctx).Create(&job).Error; err != nil {
		return nil, fmt.Errorf("create sheet import job: %w", err)
	}
	if s.importEnqueuer != nil {
		if err := s.importEnqueuer.EnqueueSheetCSVImport(ctx, job.ID); err != nil {
			failed := map[string]any{"status": ImportStatusFailed, "error": "failed to schedule import"}
			_ = s.db.WithContext(ctx).Model(&model.SheetImportJob{}).
				Where("id = ? AND org_id = ?", job.ID, orgID).
				Updates(failed).Error
			return nil, fmt.Errorf("enqueue sheet csv import: %w", err)
		}
	}
	s.PublishImportProgress(ctx, &job)
	return &job, nil
}

// GetImportJob returns one import job within the caller's org.
func (s *Service) GetImportJob(ctx context.Context, orgID, jobID uuid.UUID) (*model.SheetImportJob, error) {
	var job model.SheetImportJob
	err := s.db.WithContext(ctx).
		Where("id = ? AND org_id = ?", jobID, orgID).
		First(&job).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load sheet import job: %w", err)
	}
	return &job, nil
}
