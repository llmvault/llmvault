package sheets

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

// validateRelationTargetPage enforces that a relation field's target page
// exists, is active, and belongs to the SAME org. Cross-org relation links
// are a security bug (KI-07 class), so this runs on field create/update and
// again on every row write.
func validateRelationTargetPage(tx *gorm.DB, orgID, targetPageID uuid.UUID) error {
	var count int64
	err := tx.Model(&model.SheetPage{}).
		Where("id = ? AND org_id = ? AND archived_at IS NULL", targetPageID, orgID).
		Count(&count).Error
	if err != nil {
		return fmt.Errorf("check relation target page: %w", err)
	}
	if count == 0 {
		return &RelationError{Message: fmt.Sprintf("target page %s does not exist in this org", targetPageID)}
	}
	return nil
}

// relationBatch aggregates linked row IDs per relation field across a write
// batch so validation is one query per field, not per cell.
type relationBatch struct {
	byField map[string]*relationFieldLinks
}

type relationFieldLinks struct {
	field *model.SheetField
	ids   map[string]bool
}

func newRelationBatch() *relationBatch {
	return &relationBatch{byField: map[string]*relationFieldLinks{}}
}

func (b *relationBatch) add(field *model.SheetField, ids []string) {
	links := b.byField[field.ID]
	if links == nil {
		links = &relationFieldLinks{field: field, ids: map[string]bool{}}
		b.byField[field.ID] = links
	}
	for _, id := range ids {
		links.ids[id] = true
	}
}

// validate checks every linked row ID against the field's configured target
// page within the caller's org. Any row that is missing, cross-org, or in a
// different page rejects the whole write.
func (b *relationBatch) validate(tx *gorm.DB, orgID uuid.UUID) error {
	fieldIDs := make([]string, 0, len(b.byField))
	for id := range b.byField {
		fieldIDs = append(fieldIDs, id)
	}
	sort.Strings(fieldIDs)
	for _, fieldID := range fieldIDs {
		links := b.byField[fieldID]
		if len(links.ids) == 0 {
			continue
		}
		targetPageID, err := RelationTargetPageID(links.field.Options)
		if err != nil {
			return err
		}
		if err := validateRelationTargetPage(tx, orgID, targetPageID); err != nil {
			return &RelationError{FieldID: fieldID, Message: fmt.Sprintf("target page %s does not exist in this org", targetPageID)}
		}
		wanted := make([]string, 0, len(links.ids))
		for id := range links.ids {
			wanted = append(wanted, id)
		}
		var found []string
		err = tx.Model(&model.SheetRow{}).
			Where("id IN ? AND page_id = ? AND org_id = ?", wanted, targetPageID, orgID).
			Pluck("id", &found).Error
		if err != nil {
			return fmt.Errorf("check relation rows: %w", err)
		}
		foundSet := make(map[string]bool, len(found))
		for _, id := range found {
			foundSet[strings.ToLower(id)] = true
		}
		for _, id := range wanted {
			if !foundSet[strings.ToLower(id)] {
				return &RelationError{FieldID: fieldID, Message: fmt.Sprintf("linked row %s is not in the field's target page", id)}
			}
		}
	}
	return nil
}

// OrgAttachmentPrefix is the object-key prefix every attachment in orgID's
// cells must carry. Keys outside the org's prefix are rejected.
func OrgAttachmentPrefix(orgID uuid.UUID) string {
	return fmt.Sprintf("pub/o/%s/", orgID)
}

// validateAttachmentKeys enforces org ownership of attachment object keys.
func validateAttachmentKeys(orgID uuid.UUID, fieldID string, keys []string) error {
	prefix := OrgAttachmentPrefix(orgID)
	for _, key := range keys {
		if !strings.HasPrefix(key, prefix) || len(key) <= len(prefix) {
			return &AttachmentError{FieldID: fieldID, Key: key, Message: "object key is not owned by this org"}
		}
	}
	return nil
}

// RelationRef is a hydrated relation chip: the linked row's ID and a label
// derived from the target page's display field.
type RelationRef struct {
	ID    uuid.UUID `json:"id"`
	Label string    `json:"label"`
}

// ResolveRelations hydrates every relation link found in rows into
// {id,label} pairs with one batched lookup per target page. Labels come from
// the target page's display_field_id, falling back to its first text field.
// Archived/missing target rows are omitted (rendered as broken chips).
func (s *Service) ResolveRelations(ctx context.Context, orgID uuid.UUID, fields []model.SheetField, rows []model.SheetRow) (map[string]RelationRef, error) {
	byPage := map[uuid.UUID]map[string]bool{}
	for i := range fields {
		field := &fields[i]
		if field.Type != FieldTypeRelation {
			continue
		}
		targetPageID, err := RelationTargetPageID(field.Options)
		if err != nil {
			continue
		}
		for _, row := range rows {
			for _, id := range cellStringSlice(row.Data[field.ID]) {
				if parsed, err := uuid.Parse(id); err == nil {
					if byPage[targetPageID] == nil {
						byPage[targetPageID] = map[string]bool{}
					}
					byPage[targetPageID][parsed.String()] = true
				}
			}
		}
	}
	out := map[string]RelationRef{}
	for targetPageID, idSet := range byPage {
		if err := s.resolvePageRelations(ctx, orgID, targetPageID, idSet, out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (s *Service) resolvePageRelations(ctx context.Context, orgID, pageID uuid.UUID, idSet map[string]bool, out map[string]RelationRef) error {
	var page model.SheetPage
	err := s.db.WithContext(ctx).
		Where("id = ? AND org_id = ? AND archived_at IS NULL", pageID, orgID).
		First(&page).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil // target page gone → broken chips
	}
	if err != nil {
		return fmt.Errorf("load relation target page: %w", err)
	}
	targetFields, err := s.loadPageFields(ctx, orgID, page.ID)
	if err != nil {
		return err
	}
	displayFieldID := displayFieldFor(&page, targetFields)
	ids := make([]string, 0, len(idSet))
	for id := range idSet {
		ids = append(ids, id)
	}
	var linked []model.SheetRow
	err = s.db.WithContext(ctx).
		Where("id IN ? AND page_id = ? AND org_id = ? AND archived_at IS NULL", ids, page.ID, orgID).
		Find(&linked).Error
	if err != nil {
		return fmt.Errorf("load relation rows: %w", err)
	}
	for _, row := range linked {
		label := ""
		if displayFieldID != "" {
			label = cellString(row.Data[displayFieldID])
		}
		out[row.ID.String()] = RelationRef{ID: row.ID, Label: label}
	}
	return nil
}

// displayFieldFor resolves the label field: the page's display_field_id when
// it is an active field, else the first text field by position.
func displayFieldFor(page *model.SheetPage, fields []model.SheetField) string {
	if page.DisplayFieldID != nil {
		for _, field := range fields {
			if field.ID == *page.DisplayFieldID {
				return field.ID
			}
		}
	}
	for _, field := range fields {
		if field.Type == FieldTypeText {
			return field.ID
		}
	}
	return ""
}

func cellStringSlice(value any) []string {
	switch v := value.(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func cellString(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	default:
		return fmt.Sprintf("%v", v)
	}
}
