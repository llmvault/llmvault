package sheets

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

// Worker-facing import support (plan §2). The CSV import task drives these so
// coercion, org scoping, limits, and operation capture stay on the one
// service code path shared with REST and MCP.

// ImportJobByID loads an import job by primary key without org scoping — the
// worker's entrypoint. Every subsequent operation is scoped by the job's own
// OrgID, so a forged job ID cannot cross org boundaries.
func (s *Service) ImportJobByID(ctx context.Context, jobID uuid.UUID) (*model.SheetImportJob, error) {
	var job model.SheetImportJob
	err := s.db.WithContext(ctx).Where("id = ?", jobID).First(&job).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load sheet import job: %w", err)
	}
	return &job, nil
}

// MarkImportJobRunning transitions the job to running with the scanned total.
func (s *Service) MarkImportJobRunning(ctx context.Context, job *model.SheetImportJob, totalRows int64) error {
	updates := map[string]any{"status": ImportStatusRunning, "total_rows": totalRows, "error": ""}
	if err := s.updateImportJob(ctx, job, updates); err != nil {
		return err
	}
	job.Status = ImportStatusRunning
	job.TotalRows = totalRows
	job.Error = ""
	return nil
}

// MarkImportJobFailed records a terminal failure with a user-facing message.
func (s *Service) MarkImportJobFailed(ctx context.Context, job *model.SheetImportJob, message string) error {
	updates := map[string]any{"status": ImportStatusFailed, "error": message}
	if err := s.updateImportJob(ctx, job, updates); err != nil {
		return err
	}
	job.Status = ImportStatusFailed
	job.Error = message
	return nil
}

// ResetImportRows hard-deletes every row stamped with the job and zeroes
// processed_rows in one transaction — the rollback / retry-clean-slate
// primitive (plan §2: retries start clean).
func (s *Service) ResetImportRows(ctx context.Context, job *model.SheetImportJob) error {
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.
			Where("import_job_id = ? AND page_id = ? AND org_id = ?", job.ID, job.PageID, job.OrgID).
			Delete(&model.SheetRow{}).Error; err != nil {
			return fmt.Errorf("delete imported rows: %w", err)
		}
		return tx.Model(&model.SheetImportJob{}).
			Where("id = ? AND org_id = ?", job.ID, job.OrgID).
			Update("processed_rows", 0).Error
	})
	if err != nil {
		return fmt.Errorf("reset import rows: %w", err)
	}
	job.ProcessedRows = 0
	return nil
}

// InsertImportRows validates, coerces, and inserts one chunk of imported rows
// stamped with the job ID, bumping processed_rows in the same transaction.
// Unlike InsertRows it records no per-chunk operation — the import is undone
// as one csv_import operation captured by CompleteImportJob — and emits no
// rows_changed events; import_progress is the import's realtime channel.
func (s *Service) InsertImportRows(ctx context.Context, job *model.SheetImportJob, inserts []RowInsert) (int, error) {
	if len(inserts) == 0 {
		return 0, nil
	}
	page, fields, err := s.loadPageContext(ctx, job.OrgID, job.PageID)
	if err != nil {
		return 0, err
	}
	existing, err := s.ActiveRowCount(ctx, job.OrgID, page.ID)
	if err != nil {
		return 0, err
	}
	if int(existing)+len(inserts) > MaxRowsPerPage {
		return 0, &LimitError{Limit: "rows per page", Max: MaxRowsPerPage, Actual: int(existing) + len(inserts)}
	}
	basePosition, err := s.nextPosition(ctx, &model.SheetRow{}, "page_id = ? AND org_id = ?", page.ID, job.OrgID)
	if err != nil {
		return 0, err
	}
	batch := newRelationBatch()
	rows := make([]model.SheetRow, 0, len(inserts))
	for i, insert := range inserts {
		data, err := coerceCells(job.OrgID, fields, insert.Data, batch)
		if err != nil {
			return 0, err
		}
		data = stripNilCells(data)
		if err := checkRowSize(data); err != nil {
			return 0, err
		}
		rows = append(rows, model.SheetRow{
			PageID:           page.ID,
			OrgID:            job.OrgID,
			Data:             model.JSON(data),
			Position:         basePosition + float64(i*positionStep),
			ImportJobID:      &job.ID,
			CreatedByAgentID: job.CreatedByAgentID,
			CreatedByUserID:  job.CreatedByUserID,
		})
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := batch.validate(tx, job.OrgID); err != nil {
			return err
		}
		if err := tx.CreateInBatches(&rows, 500).Error; err != nil {
			return fmt.Errorf("insert imported rows: %w", err)
		}
		return tx.Model(&model.SheetImportJob{}).
			Where("id = ? AND org_id = ?", job.ID, job.OrgID).
			Update("processed_rows", gorm.Expr("processed_rows + ?", len(rows))).Error
	})
	if err != nil {
		return 0, err
	}
	job.ProcessedRows += int64(len(rows))
	return len(rows), nil
}

// CompleteImportJob marks the job completed and records the csv_import
// operation whose inverse ({import_job_id}) archives all imported rows.
func (s *Service) CompleteImportJob(ctx context.Context, job *model.SheetImportJob) error {
	actor := Actor{AgentID: job.CreatedByAgentID, UserID: job.CreatedByUserID}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.SheetImportJob{}).
			Where("id = ? AND org_id = ?", job.ID, job.OrgID).
			Updates(map[string]any{"status": ImportStatusCompleted, "error": ""}).Error; err != nil {
			return fmt.Errorf("complete sheet import job: %w", err)
		}
		return s.recordOperationTx(tx, &model.SheetOperation{
			OrgID:    job.OrgID,
			PageID:   job.PageID,
			Type:     OpTypeCSVImport,
			RowCount: int(job.ProcessedRows),
			Inverse:  model.JSON{"import_job_id": job.ID.String()},
		}, actor)
	})
	if err != nil {
		return err
	}
	job.Status = ImportStatusCompleted
	job.Error = ""
	return nil
}

// ImportPageFields returns the page's active fields for header mapping.
func (s *Service) ImportPageFields(ctx context.Context, orgID, pageID uuid.UUID) ([]model.SheetField, error) {
	if _, err := s.loadPage(ctx, orgID, pageID); err != nil {
		return nil, err
	}
	return s.loadPageFields(ctx, orgID, pageID)
}

// ActiveRowCount returns the page's active row count (import fail-fast and
// per-chunk MaxRowsPerPage checks).
func (s *Service) ActiveRowCount(ctx context.Context, orgID, pageID uuid.UUID) (int64, error) {
	var count int64
	err := s.db.WithContext(ctx).Model(&model.SheetRow{}).
		Where("page_id = ? AND org_id = ? AND archived_at IS NULL", pageID, orgID).
		Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("count page rows: %w", err)
	}
	return count, nil
}

func (s *Service) updateImportJob(ctx context.Context, job *model.SheetImportJob, updates map[string]any) error {
	err := s.db.WithContext(ctx).Model(&model.SheetImportJob{}).
		Where("id = ? AND org_id = ?", job.ID, job.OrgID).
		Updates(updates).Error
	if err != nil {
		return fmt.Errorf("update sheet import job: %w", err)
	}
	return nil
}
