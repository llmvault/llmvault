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

// OrgAttachmentPrefix is the object-key prefix for org-owned assets. Keys
// under it are accepted without any DB lookup.
func OrgAttachmentPrefix(orgID uuid.UUID) string {
	return fmt.Sprintf("pub/o/%s/", orgID)
}

// ValidAttachmentKey is the pure-string fast path of the shared attachment
// acceptance contract: a non-empty object under the org's pub/o/{orgID}/
// prefix with no path traversal. Any org-owned asset qualifies
// (sign(sheet_attachment) uploads land under
// pub/o/{orgID}/sheets/attachments/, but other org assets are referenceable
// too). Agent drive keys (pub/e/{agentID}/…) are NOT covered here — they are
// accepted only after the batched owner check proves the agent belongs to
// the org (stageObjectKey / Service.AuthorizeObjectKeys), so callers must
// not use this predicate alone as the full acceptance rule.
func ValidAttachmentKey(orgID uuid.UUID, key string) bool {
	prefix := OrgAttachmentPrefix(orgID)
	return strings.HasPrefix(key, prefix) &&
		len(key) > len(prefix) &&
		!strings.Contains(key, "..")
}

// agentDriveKeyOwner extracts the agent ID from a well-formed agent drive
// key: pub/e/{agentID}/{non-empty remainder} with a parseable UUID and no
// path traversal. It is a pure-string check — callers MUST still verify the
// agent belongs to the org before accepting the key.
func agentDriveKeyOwner(key string) (uuid.UUID, bool) {
	const prefix = "pub/e/"
	if !strings.HasPrefix(key, prefix) || strings.Contains(key, "..") {
		return uuid.Nil, false
	}
	rest := key[len(prefix):]
	slash := strings.IndexByte(rest, '/')
	if slash <= 0 || slash == len(rest)-1 {
		return uuid.Nil, false
	}
	agentID, err := uuid.Parse(rest[:slash])
	if err != nil {
		return uuid.Nil, false
	}
	return agentID, true
}

// stageObjectKey is the single admission rule for object keys referenced by
// sheets (attachment cells, CSV imports, download-url): org-owned
// pub/o/{orgID}/ keys pass immediately; well-formed agent drive keys
// (pub/e/{agentID}/…) are queued on the batch for the one-query "agent
// belongs to this org" check; everything else is rejected before any DB hit.
func stageObjectKey(orgID uuid.UUID, fieldID, key string, batch *relationBatch) error {
	if ValidAttachmentKey(orgID, key) {
		return nil
	}
	if agentID, ok := agentDriveKeyOwner(key); ok {
		batch.addDriveKey(agentID, fieldID, key)
		return nil
	}
	return &AttachmentError{FieldID: fieldID, Key: key, Message: "object key is not owned by this org"}
}

// validateAttachmentKeys enforces the admission rule on a cell's attachment
// keys, queueing agent-drive keys on the batch for the deferred owner check.
func validateAttachmentKeys(orgID uuid.UUID, fieldID string, keys []string, batch *relationBatch) error {
	for _, key := range keys {
		if err := stageObjectKey(orgID, fieldID, key, batch); err != nil {
			return err
		}
	}
	return nil
}

// AuthorizeObjectKey authorizes one object key for orgID: accepted when it
// is org-owned (pub/o/{orgID}/…) or an agent drive key (pub/e/{agentID}/…)
// whose agent belongs to the org. Used on write and download-url paths only
// — never per-row reads.
func (s *Service) AuthorizeObjectKey(ctx context.Context, orgID uuid.UUID, key string) error {
	return s.AuthorizeObjectKeys(ctx, orgID, []string{key})
}

// AuthorizeObjectKeys authorizes a batch of object keys for orgID with at
// most one agents-table lookup. Malformed keys are rejected on the pure
// string fast path before any DB hit.
func (s *Service) AuthorizeObjectKeys(ctx context.Context, orgID uuid.UUID, keys []string) error {
	batch := newRelationBatch()
	for _, key := range keys {
		if err := stageObjectKey(orgID, "", key, batch); err != nil {
			return err
		}
	}
	return batch.validateDriveOwners(s.db.WithContext(ctx), orgID)
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
