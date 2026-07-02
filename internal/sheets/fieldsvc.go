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

// createFieldTx validates and creates one field inside an open transaction.
// It does not capture a field_change operation — callers creating fields on
// an established page do that; inline sheet/page creation does not.
func (s *Service) createFieldTx(tx *gorm.DB, page *model.SheetPage, spec FieldSpec, position float64) (*model.SheetField, error) {
	name := strings.TrimSpace(spec.Name)
	if name == "" {
		return nil, &OptionsError{FieldType: spec.Type, Message: "field name is required"}
	}
	if !KnownFieldType(spec.Type) {
		return nil, &UnknownFieldTypeError{Type: spec.Type}
	}
	options := spec.Options
	if options == nil {
		options = model.JSON{}
	}
	if err := ValidateFieldOptions(spec.Type, options); err != nil {
		return nil, err
	}
	if spec.Type == FieldTypeRelation {
		targetPageID, err := RelationTargetPageID(options)
		if err != nil {
			return nil, err
		}
		if err := validateRelationTargetPage(tx, page.OrgID, targetPageID); err != nil {
			return nil, err
		}
	}
	id, err := NewFieldID()
	if err != nil {
		return nil, err
	}
	field := model.SheetField{
		ID:       id,
		PageID:   page.ID,
		OrgID:    page.OrgID,
		Name:     name,
		Type:     spec.Type,
		Options:  options,
		Position: position,
	}
	if err := tx.Create(&field).Error; err != nil {
		return nil, fmt.Errorf("create sheet field: %w", err)
	}
	if page.DisplayFieldID == nil && field.Type == FieldTypeText {
		if err := tx.Model(&model.SheetPage{}).
			Where("id = ? AND org_id = ?", page.ID, page.OrgID).
			Update("display_field_id", field.ID).Error; err != nil {
			return nil, fmt.Errorf("default display field: %w", err)
		}
		page.DisplayFieldID = &field.ID
	}
	return &field, nil
}

// CreateField adds a column to an existing page and records a field_change
// operation whose inverse archives the new field.
func (s *Service) CreateField(ctx context.Context, orgID, pageID uuid.UUID, spec FieldSpec, actor Actor) (*model.SheetField, error) {
	page, err := s.loadPage(ctx, orgID, pageID)
	if err != nil {
		return nil, err
	}
	var count int64
	if err := s.db.WithContext(ctx).Model(&model.SheetField{}).
		Where("page_id = ? AND org_id = ? AND archived_at IS NULL", page.ID, orgID).
		Count(&count).Error; err != nil {
		return nil, fmt.Errorf("count sheet fields: %w", err)
	}
	if count >= MaxFieldsPerPage {
		return nil, &LimitError{Limit: "fields per page", Max: MaxFieldsPerPage, Actual: int(count) + 1}
	}
	position, err := s.nextPosition(ctx, &model.SheetField{}, "page_id = ? AND org_id = ?", page.ID, orgID)
	if err != nil {
		return nil, err
	}
	var field *model.SheetField
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		field, err = s.createFieldTx(tx, page, spec, position)
		if err != nil {
			return err
		}
		return s.recordOperationTx(tx, &model.SheetOperation{
			OrgID:   orgID,
			PageID:  page.ID,
			Type:    OpTypeFieldChange,
			Inverse: model.JSON{"field_id": field.ID, "action": "archive"},
		}, actor)
	})
	if err != nil {
		return nil, err
	}
	s.publishFieldsChanged(ctx, orgID, page.ID, field.ID, "create", actor)
	return field, nil
}

type UpdateFieldRequest struct {
	Name     *string
	Type     *string
	Options  *model.JSON
	Position *float64
}

// UpdateField renames, retypes, repositions, or reconfigures a column,
// recording a field_change operation carrying the prior definition.
func (s *Service) UpdateField(ctx context.Context, orgID uuid.UUID, fieldID string, req UpdateFieldRequest, actor Actor) (*model.SheetField, error) {
	field, err := s.loadField(ctx, orgID, fieldID)
	if err != nil {
		return nil, err
	}
	prior := fieldSnapshot(field)
	if req.Name != nil {
		if name := strings.TrimSpace(*req.Name); name != "" {
			field.Name = name
		}
	}
	if req.Type != nil {
		field.Type = *req.Type
	}
	if req.Options != nil {
		field.Options = *req.Options
	}
	if req.Position != nil {
		field.Position = *req.Position
	}
	if !KnownFieldType(field.Type) {
		return nil, &UnknownFieldTypeError{Type: field.Type}
	}
	if field.Options == nil {
		field.Options = model.JSON{}
	}
	if err := ValidateFieldOptions(field.Type, field.Options); err != nil {
		return nil, err
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if field.Type == FieldTypeRelation {
			targetPageID, err := RelationTargetPageID(field.Options)
			if err != nil {
				return err
			}
			if err := validateRelationTargetPage(tx, orgID, targetPageID); err != nil {
				return err
			}
		}
		updates := map[string]any{
			"name": field.Name, "type": field.Type,
			"options": field.Options, "position": field.Position,
		}
		if err := tx.Model(&model.SheetField{}).
			Where("id = ? AND org_id = ?", field.ID, orgID).
			Updates(updates).Error; err != nil {
			return fmt.Errorf("update sheet field: %w", err)
		}
		return s.recordOperationTx(tx, &model.SheetOperation{
			OrgID:   orgID,
			PageID:  field.PageID,
			Type:    OpTypeFieldChange,
			Inverse: model.JSON{"field_id": field.ID, "action": "restore", "prior": prior},
		}, actor)
	})
	if err != nil {
		return nil, err
	}
	s.publishFieldsChanged(ctx, orgID, field.PageID, field.ID, "update", actor)
	return field, nil
}

// ArchiveField soft-deletes a column. Rows are untouched — cells key by
// field ID — so the recorded inverse fully restores the column.
func (s *Service) ArchiveField(ctx context.Context, orgID uuid.UUID, fieldID string, actor Actor) error {
	field, err := s.loadField(ctx, orgID, fieldID)
	if err != nil {
		return err
	}
	prior := fieldSnapshot(field)
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.SheetField{}).
			Where("id = ? AND org_id = ? AND archived_at IS NULL", field.ID, orgID).
			Update("archived_at", time.Now().UTC()).Error; err != nil {
			return fmt.Errorf("archive sheet field: %w", err)
		}
		if err := tx.Model(&model.SheetPage{}).
			Where("id = ? AND org_id = ? AND display_field_id = ?", field.PageID, orgID, field.ID).
			Update("display_field_id", nil).Error; err != nil {
			return fmt.Errorf("clear display field: %w", err)
		}
		return s.recordOperationTx(tx, &model.SheetOperation{
			OrgID:   orgID,
			PageID:  field.PageID,
			Type:    OpTypeFieldChange,
			Inverse: model.JSON{"field_id": field.ID, "action": "restore", "prior": prior},
		}, actor)
	})
	if err != nil {
		return err
	}
	s.publishFieldsChanged(ctx, orgID, field.PageID, field.ID, "delete", actor)
	return nil
}

func (s *Service) loadField(ctx context.Context, orgID uuid.UUID, fieldID string) (*model.SheetField, error) {
	var field model.SheetField
	err := s.db.WithContext(ctx).
		Where("id = ? AND org_id = ? AND archived_at IS NULL", fieldID, orgID).
		First(&field).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load sheet field: %w", err)
	}
	return &field, nil
}

// fieldSnapshot captures a field definition for a field_change inverse.
func fieldSnapshot(field *model.SheetField) map[string]any {
	return map[string]any{
		"name":     field.Name,
		"type":     field.Type,
		"options":  map[string]any(field.Options),
		"position": field.Position,
	}
}
