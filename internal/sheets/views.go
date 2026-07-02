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

var viewTypes = map[string]bool{"grid": true, "kanban": true, "gallery": true, "calendar": true}

type ViewSpec struct {
	Name   string
	Type   string
	Config model.JSON
}

func (s *Service) CreateView(ctx context.Context, orgID, pageID uuid.UUID, spec ViewSpec) (*model.SheetView, error) {
	page, err := s.loadPage(ctx, orgID, pageID)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(spec.Name)
	if name == "" {
		name = "Grid"
	}
	viewType := spec.Type
	if viewType == "" {
		viewType = "grid"
	}
	if !viewTypes[viewType] {
		return nil, fmt.Errorf("sheets: unknown view type %q", viewType)
	}
	config := spec.Config
	if config == nil {
		config = model.JSON{}
	}
	position, err := s.nextPosition(ctx, &model.SheetView{}, "page_id = ? AND org_id = ?", page.ID, orgID)
	if err != nil {
		return nil, err
	}
	view := model.SheetView{
		PageID:   page.ID,
		OrgID:    orgID,
		Name:     name,
		Type:     viewType,
		Config:   config,
		Position: position,
	}
	if err := s.db.WithContext(ctx).Create(&view).Error; err != nil {
		return nil, fmt.Errorf("create sheet view: %w", err)
	}
	return &view, nil
}

func (s *Service) ListViews(ctx context.Context, orgID, pageID uuid.UUID) ([]model.SheetView, error) {
	var views []model.SheetView
	err := s.db.WithContext(ctx).
		Where("page_id = ? AND org_id = ? AND archived_at IS NULL", pageID, orgID).
		Order("position ASC").Find(&views).Error
	if err != nil {
		return nil, fmt.Errorf("list sheet views: %w", err)
	}
	return views, nil
}

type UpdateViewRequest struct {
	Name     *string
	Config   *model.JSON
	Position *float64
}

func (s *Service) UpdateView(ctx context.Context, orgID, viewID uuid.UUID, req UpdateViewRequest) (*model.SheetView, error) {
	var view model.SheetView
	err := s.db.WithContext(ctx).
		Where("id = ? AND org_id = ? AND archived_at IS NULL", viewID, orgID).
		First(&view).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load sheet view: %w", err)
	}
	updates := map[string]any{}
	if req.Name != nil {
		if name := strings.TrimSpace(*req.Name); name != "" {
			updates["name"] = name
		}
	}
	if req.Config != nil {
		updates["config"] = *req.Config
	}
	if req.Position != nil {
		updates["position"] = *req.Position
	}
	if len(updates) == 0 {
		return &view, nil
	}
	if err := s.db.WithContext(ctx).Model(&view).
		Where("org_id = ?", orgID).
		Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("update sheet view: %w", err)
	}
	return &view, nil
}

func (s *Service) ArchiveView(ctx context.Context, orgID, viewID uuid.UUID) error {
	result := s.db.WithContext(ctx).Model(&model.SheetView{}).
		Where("id = ? AND org_id = ? AND archived_at IS NULL", viewID, orgID).
		Update("archived_at", time.Now().UTC())
	if result.Error != nil {
		return fmt.Errorf("archive sheet view: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
