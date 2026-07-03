package sheets

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

// GetSheet loads one sheet with all active pages and fields.
func (s *Service) GetSheet(ctx context.Context, orgID, sheetID uuid.UUID) (*SheetStructure, error) {
	sheet, err := s.loadSheet(ctx, orgID, sheetID)
	if err != nil {
		return nil, err
	}
	var pages []model.SheetPage
	if err := s.db.WithContext(ctx).
		Where("sheet_id = ? AND org_id = ? AND archived_at IS NULL", sheet.ID, orgID).
		Order("position ASC").Find(&pages).Error; err != nil {
		return nil, fmt.Errorf("list sheet pages: %w", err)
	}
	structure := &SheetStructure{Sheet: *sheet}
	for _, page := range pages {
		fields, err := s.loadPageFields(ctx, orgID, page.ID)
		if err != nil {
			return nil, err
		}
		structure.Pages = append(structure.Pages, PageStructure{Page: page, Fields: fields})
	}
	return structure, nil
}

type UpdateSheetRequest struct {
	Name        *string
	Description *string
	Icon        *string
}

func (s *Service) UpdateSheet(ctx context.Context, orgID, sheetID uuid.UUID, req UpdateSheetRequest) (*model.Sheet, error) {
	sheet, err := s.loadSheet(ctx, orgID, sheetID)
	if err != nil {
		return nil, err
	}
	updates := map[string]any{}
	if req.Name != nil {
		if name := strings.TrimSpace(*req.Name); name != "" {
			updates["name"] = name
		}
	}
	if req.Description != nil {
		updates["description"] = strings.TrimSpace(*req.Description)
	}
	if req.Icon != nil {
		updates["icon"] = strings.TrimSpace(*req.Icon)
	}
	if len(updates) == 0 {
		return sheet, nil
	}
	if err := s.db.WithContext(ctx).Model(sheet).
		Where("org_id = ?", orgID).
		Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("update sheet: %w", err)
	}
	return sheet, nil
}

func (s *Service) ArchiveSheet(ctx context.Context, orgID, sheetID uuid.UUID) error {
	result := s.db.WithContext(ctx).Model(&model.Sheet{}).
		Where("id = ? AND org_id = ? AND archived_at IS NULL", sheetID, orgID).
		Update("archived_at", time.Now().UTC())
	if result.Error != nil {
		return fmt.Errorf("archive sheet: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// SheetByID loads one active sheet org-scoped, without pages or fields.
func (s *Service) SheetByID(ctx context.Context, orgID, sheetID uuid.UUID) (*model.Sheet, error) {
	return s.loadSheet(ctx, orgID, sheetID)
}

func (s *Service) loadSheet(ctx context.Context, orgID, sheetID uuid.UUID) (*model.Sheet, error) {
	var sheet model.Sheet
	err := s.db.WithContext(ctx).
		Where("id = ? AND org_id = ? AND archived_at IS NULL", sheetID, orgID).
		First(&sheet).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load sheet: %w", err)
	}
	return &sheet, nil
}

var slugPattern = regexp.MustCompile(`[^a-z0-9]+`)

func normalizeSlug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = slugPattern.ReplaceAllString(value, "-")
	return strings.Trim(value, "-")
}

func (s *Service) availableSheetSlug(ctx context.Context, orgID uuid.UUID, base string) (string, error) {
	if base == "" {
		base = "sheet"
	}
	for i := 0; i < 1000; i++ {
		slug := base
		if i > 0 {
			slug = fmt.Sprintf("%s-%d", base, i)
		}
		var count int64
		err := s.db.WithContext(ctx).Model(&model.Sheet{}).
			Where("org_id = ? AND slug = ? AND archived_at IS NULL", orgID, slug).
			Count(&count).Error
		if err != nil {
			return "", fmt.Errorf("check sheet slug: %w", err)
		}
		if count == 0 {
			return slug, nil
		}
	}
	return "", fmt.Errorf("could not allocate sheet slug")
}
