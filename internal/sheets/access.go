package sheets

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

// Channel-scope guards. Sheets belong to exactly one channel and their
// visibility follows that channel's access. Every by-ID entry point (sheet,
// page, field, import job, operation) resolves back to the parent sheet's
// channel; these guards enforce that the target lives in the caller's channel,
// returning ErrNotFound on a mismatch so a cross-channel ID is indistinguishable
// from a missing one (no existence leak). The MCP tools call them with the
// channel derived from the agent's session; the REST handlers authorize channel
// access separately via handler.canUseChannel.

// SheetInChannel verifies the sheet exists in the org and belongs to channelID.
func (s *Service) SheetInChannel(ctx context.Context, orgID, channelID, sheetID uuid.UUID) error {
	return s.channelGuard(s.db.WithContext(ctx).
		Model(&model.Sheet{}).
		Where("sheets.id = ? AND sheets.org_id = ? AND sheets.channel_id = ? AND sheets.archived_at IS NULL",
			sheetID, orgID, channelID))
}

// PageInChannel verifies the page's sheet belongs to channelID.
func (s *Service) PageInChannel(ctx context.Context, orgID, channelID, pageID uuid.UUID) error {
	return s.channelGuard(s.db.WithContext(ctx).
		Model(&model.SheetPage{}).
		Joins("JOIN sheets ON sheets.id = sheet_pages.sheet_id").
		Where("sheet_pages.id = ? AND sheet_pages.org_id = ? AND sheets.channel_id = ? AND sheet_pages.archived_at IS NULL",
			pageID, orgID, channelID))
}

// PageInSheet verifies the page belongs to sheetID and that both the page and
// its sheet are active. The internal app API calls this keyed on the app's
// bound sheet, so a page of any other sheet — same org included — is
// indistinguishable from a missing one.
func (s *Service) PageInSheet(ctx context.Context, orgID, sheetID, pageID uuid.UUID) error {
	return s.channelGuard(s.db.WithContext(ctx).
		Model(&model.SheetPage{}).
		Joins("JOIN sheets ON sheets.id = sheet_pages.sheet_id AND sheets.archived_at IS NULL").
		Where("sheet_pages.id = ? AND sheet_pages.org_id = ? AND sheets.id = ? AND sheet_pages.archived_at IS NULL",
			pageID, orgID, sheetID))
}

// FieldInChannel verifies the field's page's sheet belongs to channelID.
func (s *Service) FieldInChannel(ctx context.Context, orgID, channelID uuid.UUID, fieldID string) error {
	return s.channelGuard(s.db.WithContext(ctx).
		Model(&model.SheetField{}).
		Joins("JOIN sheet_pages ON sheet_pages.id = sheet_fields.page_id").
		Joins("JOIN sheets ON sheets.id = sheet_pages.sheet_id").
		Where("sheet_fields.id = ? AND sheet_fields.org_id = ? AND sheets.channel_id = ? AND sheet_fields.archived_at IS NULL",
			fieldID, orgID, channelID))
}

// FieldInPage verifies the field belongs to pageID and both are active. The
// REST layer, having already authorized the addressed sheet's channel, calls
// this keyed on the path's pageID so a field of any other page — same org
// included — is indistinguishable from a missing one.
func (s *Service) FieldInPage(ctx context.Context, orgID, pageID uuid.UUID, fieldID string) error {
	return s.channelGuard(s.db.WithContext(ctx).
		Model(&model.SheetField{}).
		Where("sheet_fields.id = ? AND sheet_fields.org_id = ? AND sheet_fields.page_id = ? AND sheet_fields.archived_at IS NULL",
			fieldID, orgID, pageID))
}

// ViewInPage verifies the view belongs to pageID and both are active. The REST
// layer, having already authorized the addressed sheet's channel, calls this
// keyed on the path's pageID so a view of any other page — same org included —
// is indistinguishable from a missing one.
func (s *Service) ViewInPage(ctx context.Context, orgID, pageID, viewID uuid.UUID) error {
	return s.channelGuard(s.db.WithContext(ctx).
		Model(&model.SheetView{}).
		Where("sheet_views.id = ? AND sheet_views.org_id = ? AND sheet_views.page_id = ? AND sheet_views.archived_at IS NULL",
			viewID, orgID, pageID))
}

// ImportJobInChannel verifies the import job's page's sheet belongs to channelID.
func (s *Service) ImportJobInChannel(ctx context.Context, orgID, channelID, jobID uuid.UUID) error {
	return s.channelGuard(s.db.WithContext(ctx).
		Model(&model.SheetImportJob{}).
		Joins("JOIN sheet_pages ON sheet_pages.id = sheet_import_jobs.page_id").
		Joins("JOIN sheets ON sheets.id = sheet_pages.sheet_id").
		Where("sheet_import_jobs.id = ? AND sheet_import_jobs.org_id = ? AND sheets.channel_id = ?",
			jobID, orgID, channelID))
}

// OperationInChannel verifies the operation's page's sheet belongs to channelID.
func (s *Service) OperationInChannel(ctx context.Context, orgID, channelID, operationID uuid.UUID) error {
	return s.channelGuard(s.db.WithContext(ctx).
		Model(&model.SheetOperation{}).
		Joins("JOIN sheet_pages ON sheet_pages.id = sheet_operations.page_id").
		Joins("JOIN sheets ON sheets.id = sheet_pages.sheet_id").
		Where("sheet_operations.id = ? AND sheet_operations.org_id = ? AND sheets.channel_id = ?",
			operationID, orgID, channelID))
}

// ChannelForSheet returns the channel a sheet belongs to, org-scoped. Used by
// the REST layer to authorize channel access for an already-addressed sheet.
func (s *Service) ChannelForSheet(ctx context.Context, orgID, sheetID uuid.UUID) (uuid.UUID, error) {
	sheet, err := s.loadSheet(ctx, orgID, sheetID)
	if err != nil {
		return uuid.Nil, err
	}
	return sheet.ChannelID, nil
}

// ChannelForImportJob returns the channel of the sheet owning an import job,
// org-scoped. Used by the REST /sheets/imports/{jobID} route, which has no
// sheetID in its path, to authorize channel access. Returns ErrNotFound when the
// job does not exist in the org.
func (s *Service) ChannelForImportJob(ctx context.Context, orgID, jobID uuid.UUID) (uuid.UUID, error) {
	var row struct{ ChannelID uuid.UUID }
	err := s.db.WithContext(ctx).
		Model(&model.SheetImportJob{}).
		Select("sheets.channel_id AS channel_id").
		Joins("JOIN sheet_pages ON sheet_pages.id = sheet_import_jobs.page_id").
		Joins("JOIN sheets ON sheets.id = sheet_pages.sheet_id").
		Where("sheet_import_jobs.id = ? AND sheet_import_jobs.org_id = ?", jobID, orgID).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return uuid.Nil, ErrNotFound
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("channel for import job: %w", err)
	}
	return row.ChannelID, nil
}

// channelGuard runs an existence query and maps "no such row" to ErrNotFound.
func (s *Service) channelGuard(q *gorm.DB) error {
	var count int64
	if err := q.Count(&count).Error; err != nil {
		return fmt.Errorf("sheets channel guard: %w", err)
	}
	if count == 0 {
		return ErrNotFound
	}
	return nil
}
