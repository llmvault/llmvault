package sheets

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

// Team-scope guards. Sheets belong to exactly one team and their
// visibility follows that team's access. Every by-ID entry point (sheet,
// page, field, import job, operation) resolves back to the parent sheet's
// team; these guards enforce that the target lives in the caller's team,
// returning ErrNotFound on a mismatch so a cross-team ID is indistinguishable
// from a missing one (no existence leak). The MCP tools call them with the
// team derived from the agent's session; the REST handlers authorize team
// access separately via the handler's team authorization helper.

// SheetInTeam verifies the sheet exists in the org and belongs to teamID.
func (s *Service) SheetInTeam(ctx context.Context, orgID, teamID, sheetID uuid.UUID) error {
	return s.teamGuard(s.db.WithContext(ctx).
		Model(&model.Sheet{}).
		Where("sheets.id = ? AND sheets.org_id = ? AND sheets.team_id = ? AND sheets.archived_at IS NULL",
			sheetID, orgID, teamID))
}

// PageInTeam verifies the page's sheet belongs to teamID.
func (s *Service) PageInTeam(ctx context.Context, orgID, teamID, pageID uuid.UUID) error {
	return s.teamGuard(s.db.WithContext(ctx).
		Model(&model.SheetPage{}).
		Joins("JOIN sheets ON sheets.id = sheet_pages.sheet_id").
		Where("sheet_pages.id = ? AND sheet_pages.org_id = ? AND sheets.team_id = ? AND sheet_pages.archived_at IS NULL",
			pageID, orgID, teamID))
}

// PageInSheet verifies the page belongs to sheetID and that both the page and
// its sheet are active. The internal app API calls this keyed on the app's
// bound sheet, so a page of any other sheet — same org included — is
// indistinguishable from a missing one.
func (s *Service) PageInSheet(ctx context.Context, orgID, sheetID, pageID uuid.UUID) error {
	return s.teamGuard(s.db.WithContext(ctx).
		Model(&model.SheetPage{}).
		Joins("JOIN sheets ON sheets.id = sheet_pages.sheet_id AND sheets.archived_at IS NULL").
		Where("sheet_pages.id = ? AND sheet_pages.org_id = ? AND sheets.id = ? AND sheet_pages.archived_at IS NULL",
			pageID, orgID, sheetID))
}

// FieldInTeam verifies the field's page's sheet belongs to teamID.
func (s *Service) FieldInTeam(ctx context.Context, orgID, teamID uuid.UUID, fieldID string) error {
	return s.teamGuard(s.db.WithContext(ctx).
		Model(&model.SheetField{}).
		Joins("JOIN sheet_pages ON sheet_pages.id = sheet_fields.page_id").
		Joins("JOIN sheets ON sheets.id = sheet_pages.sheet_id").
		Where("sheet_fields.id = ? AND sheet_fields.org_id = ? AND sheets.team_id = ? AND sheet_fields.archived_at IS NULL",
			fieldID, orgID, teamID))
}

// FieldInPage verifies the field belongs to pageID and both are active. The
// REST layer, having already authorized the addressed sheet's team, calls
// this keyed on the path's pageID so a field of any other page — same org
// included — is indistinguishable from a missing one.
func (s *Service) FieldInPage(ctx context.Context, orgID, pageID uuid.UUID, fieldID string) error {
	return s.teamGuard(s.db.WithContext(ctx).
		Model(&model.SheetField{}).
		Where("sheet_fields.id = ? AND sheet_fields.org_id = ? AND sheet_fields.page_id = ? AND sheet_fields.archived_at IS NULL",
			fieldID, orgID, pageID))
}

// ViewInPage verifies the view belongs to pageID and both are active. The REST
// layer, having already authorized the addressed sheet's team, calls this
// keyed on the path's pageID so a view of any other page — same org included —
// is indistinguishable from a missing one.
func (s *Service) ViewInPage(ctx context.Context, orgID, pageID, viewID uuid.UUID) error {
	return s.teamGuard(s.db.WithContext(ctx).
		Model(&model.SheetView{}).
		Where("sheet_views.id = ? AND sheet_views.org_id = ? AND sheet_views.page_id = ? AND sheet_views.archived_at IS NULL",
			viewID, orgID, pageID))
}

// ImportJobInTeam verifies the import job's page's sheet belongs to teamID.
func (s *Service) ImportJobInTeam(ctx context.Context, orgID, teamID, jobID uuid.UUID) error {
	return s.teamGuard(s.db.WithContext(ctx).
		Model(&model.SheetImportJob{}).
		Joins("JOIN sheet_pages ON sheet_pages.id = sheet_import_jobs.page_id").
		Joins("JOIN sheets ON sheets.id = sheet_pages.sheet_id").
		Where("sheet_import_jobs.id = ? AND sheet_import_jobs.org_id = ? AND sheets.team_id = ?",
			jobID, orgID, teamID))
}

// OperationInTeam verifies the operation's page's sheet belongs to teamID.
func (s *Service) OperationInTeam(ctx context.Context, orgID, teamID, operationID uuid.UUID) error {
	return s.teamGuard(s.db.WithContext(ctx).
		Model(&model.SheetOperation{}).
		Joins("JOIN sheet_pages ON sheet_pages.id = sheet_operations.page_id").
		Joins("JOIN sheets ON sheets.id = sheet_pages.sheet_id").
		Where("sheet_operations.id = ? AND sheet_operations.org_id = ? AND sheets.team_id = ?",
			operationID, orgID, teamID))
}

// TeamForSheet returns the team a sheet belongs to, org-scoped.
func (s *Service) TeamForSheet(ctx context.Context, orgID, sheetID uuid.UUID) (uuid.UUID, error) {
	sheet, err := s.loadSheet(ctx, orgID, sheetID)
	if err != nil {
		return uuid.Nil, err
	}
	return sheet.TeamID, nil
}

// TeamForImportJob returns the team of the sheet owning an import job,
// org-scoped. Returns ErrNotFound when the
// job does not exist in the org.
func (s *Service) TeamForImportJob(ctx context.Context, orgID, jobID uuid.UUID) (uuid.UUID, error) {
	var row struct{ TeamID uuid.UUID }
	err := s.db.WithContext(ctx).
		Model(&model.SheetImportJob{}).
		Select("sheets.team_id AS team_id").
		Joins("JOIN sheet_pages ON sheet_pages.id = sheet_import_jobs.page_id").
		Joins("JOIN sheets ON sheets.id = sheet_pages.sheet_id").
		Where("sheet_import_jobs.id = ? AND sheet_import_jobs.org_id = ?", jobID, orgID).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return uuid.Nil, ErrNotFound
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("team for import job: %w", err)
	}
	return row.TeamID, nil
}

// teamGuard runs an existence query and maps "no such row" to ErrNotFound.
func (s *Service) teamGuard(q *gorm.DB) error {
	var count int64
	if err := q.Count(&count).Error; err != nil {
		return fmt.Errorf("sheets team guard: %w", err)
	}
	if count == 0 {
		return ErrNotFound
	}
	return nil
}
