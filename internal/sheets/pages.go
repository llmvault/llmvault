package sheets

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

// createPageTx creates a page and its inline fields inside an open
// transaction. Positions for fields are appended in spec order and the
// page's display field defaults to its first text field.
func (s *Service) createPageTx(tx *gorm.DB, sheet *model.Sheet, spec PageSpec, position float64) (*model.SheetPage, []model.SheetField, error) {
	name := strings.TrimSpace(spec.Name)
	if name == "" {
		name = "Page 1"
	}
	if len(spec.Fields) > MaxFieldsPerPage {
		return nil, nil, &LimitError{Limit: "fields per page", Max: MaxFieldsPerPage, Actual: len(spec.Fields)}
	}
	page := model.SheetPage{
		SheetID:  sheet.ID,
		OrgID:    sheet.OrgID,
		Name:     name,
		Position: position,
	}
	if err := tx.Create(&page).Error; err != nil {
		return nil, nil, fmt.Errorf("create sheet page: %w", err)
	}
	fields := make([]model.SheetField, 0, len(spec.Fields))
	for i, fieldSpec := range spec.Fields {
		field, err := s.createFieldTx(tx, &page, fieldSpec, float64((i+1)*positionStep))
		if err != nil {
			return nil, nil, err
		}
		fields = append(fields, *field)
	}
	return &page, fields, nil
}

// CreatePage adds a tab to a sheet.
func (s *Service) CreatePage(ctx context.Context, orgID, sheetID uuid.UUID, spec PageSpec) (*PageStructure, error) {
	sheet, err := s.loadSheet(ctx, orgID, sheetID)
	if err != nil {
		return nil, err
	}
	var count int64
	if err := s.db.WithContext(ctx).Model(&model.SheetPage{}).
		Where("sheet_id = ? AND org_id = ? AND archived_at IS NULL", sheet.ID, orgID).
		Count(&count).Error; err != nil {
		return nil, fmt.Errorf("count sheet pages: %w", err)
	}
	if count >= MaxPagesPerSheet {
		return nil, &LimitError{Limit: "pages per sheet", Max: MaxPagesPerSheet, Actual: int(count) + 1}
	}
	position, err := s.nextPosition(ctx, &model.SheetPage{}, "sheet_id = ? AND org_id = ?", sheet.ID, orgID)
	if err != nil {
		return nil, err
	}
	var structure *PageStructure
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		page, fields, err := s.createPageTx(tx, sheet, spec, position)
		if err != nil {
			return err
		}
		structure = &PageStructure{Page: *page, Fields: fields}
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.publishPagesChanged(ctx, sheet.ID, structure.Page.ID, "create")
	return structure, nil
}

type UpdatePageRequest struct {
	Name           *string
	Position       *float64
	DisplayFieldID *string // empty string clears the display field
}

func (s *Service) UpdatePage(ctx context.Context, orgID, pageID uuid.UUID, req UpdatePageRequest) (*model.SheetPage, error) {
	page, err := s.loadPage(ctx, orgID, pageID)
	if err != nil {
		return nil, err
	}
	updates := map[string]any{}
	if req.Name != nil {
		if name := strings.TrimSpace(*req.Name); name != "" {
			updates["name"] = name
		}
	}
	if req.Position != nil {
		updates["position"] = *req.Position
	}
	if req.DisplayFieldID != nil {
		if *req.DisplayFieldID == "" {
			updates["display_field_id"] = nil
		} else {
			var count int64
			err := s.db.WithContext(ctx).Model(&model.SheetField{}).
				Where("id = ? AND page_id = ? AND org_id = ? AND archived_at IS NULL", *req.DisplayFieldID, page.ID, orgID).
				Count(&count).Error
			if err != nil {
				return nil, fmt.Errorf("check display field: %w", err)
			}
			if count == 0 {
				return nil, &UnknownFieldError{FieldID: *req.DisplayFieldID}
			}
			updates["display_field_id"] = *req.DisplayFieldID
		}
	}
	if len(updates) == 0 {
		return page, nil
	}
	if err := s.db.WithContext(ctx).Model(page).
		Where("org_id = ?", orgID).
		Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("update sheet page: %w", err)
	}
	s.publishPagesChanged(ctx, page.SheetID, page.ID, "update")
	return page, nil
}

func (s *Service) ArchivePage(ctx context.Context, orgID, pageID uuid.UUID) error {
	page, err := s.loadPage(ctx, orgID, pageID)
	if err != nil {
		return err
	}
	result := s.db.WithContext(ctx).Model(&model.SheetPage{}).
		Where("id = ? AND org_id = ? AND archived_at IS NULL", pageID, orgID).
		Update("archived_at", time.Now().UTC())
	if result.Error != nil {
		return fmt.Errorf("archive sheet page: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	s.publishPagesChanged(ctx, page.SheetID, page.ID, "delete")
	return nil
}

func (s *Service) loadPage(ctx context.Context, orgID, pageID uuid.UUID) (*model.SheetPage, error) {
	var page model.SheetPage
	err := s.db.WithContext(ctx).
		Where("id = ? AND org_id = ? AND archived_at IS NULL", pageID, orgID).
		First(&page).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load sheet page: %w", err)
	}
	return &page, nil
}

func (s *Service) loadPageFields(ctx context.Context, orgID, pageID uuid.UUID) ([]model.SheetField, error) {
	var fields []model.SheetField
	err := s.db.WithContext(ctx).
		Where("page_id = ? AND org_id = ? AND archived_at IS NULL", pageID, orgID).
		Order("position ASC").Find(&fields).Error
	if err != nil {
		return nil, fmt.Errorf("list sheet fields: %w", err)
	}
	return fields, nil
}

// PageRowCounts returns active-row counts per page for a sheet's bootstrap
// response, in one grouped query.
func (s *Service) PageRowCounts(ctx context.Context, orgID uuid.UUID, pageIDs []uuid.UUID) (map[uuid.UUID]int64, error) {
	out := make(map[uuid.UUID]int64, len(pageIDs))
	if len(pageIDs) == 0 {
		return out, nil
	}
	type pageCount struct {
		PageID uuid.UUID
		Count  int64
	}
	var counts []pageCount
	err := s.db.WithContext(ctx).Model(&model.SheetRow{}).
		Select("page_id, COUNT(*) AS count").
		Where("page_id IN ? AND org_id = ? AND archived_at IS NULL", pageIDs, orgID).
		Group("page_id").Scan(&counts).Error
	if err != nil {
		return nil, fmt.Errorf("count page rows: %w", err)
	}
	for _, c := range counts {
		out[c.PageID] = c.Count
	}
	return out, nil
}

// nextPosition returns an appended fractional-index position for the scope.
func (s *Service) nextPosition(ctx context.Context, mdl any, where string, args ...any) (float64, error) {
	var max *float64
	err := s.db.WithContext(ctx).Model(mdl).
		Where(where, args...).
		Select("MAX(position)").Scan(&max).Error
	if err != nil {
		return 0, fmt.Errorf("next position: %w", err)
	}
	if max == nil {
		return positionStep, nil
	}
	return *max + positionStep, nil
}
