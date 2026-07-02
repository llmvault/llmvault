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

// relationBatch aggregates linked row IDs per relation field and pending
// agent-drive attachment keys across a write batch so validation is one
// query per field (relations) plus at most one query total (drive owners),
// not per cell.
type relationBatch struct {
	byField map[string]*relationFieldLinks
	// driveKeys queues well-formed pub/e/{agentID}/… attachment keys for the
	// batched "agent belongs to this org" ownership check.
	driveKeys map[uuid.UUID][]agentDriveRef
}

type relationFieldLinks struct {
	field *model.SheetField
	ids   map[string]bool
}

// agentDriveRef is one queued agent-drive attachment key, kept with its
// field ID so ownership failures report the offending cell.
type agentDriveRef struct {
	fieldID string
	key     string
}

func newRelationBatch() *relationBatch {
	return &relationBatch{
		byField:   map[string]*relationFieldLinks{},
		driveKeys: map[uuid.UUID][]agentDriveRef{},
	}
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

func (b *relationBatch) addDriveKey(agentID uuid.UUID, fieldID, key string) {
	b.driveKeys[agentID] = append(b.driveKeys[agentID], agentDriveRef{fieldID: fieldID, key: key})
}

// validate checks every linked row ID against the field's configured target
// page within the caller's org, and every queued agent-drive attachment key
// against the agents table. Any row that is missing, cross-org, or in a
// different page — or any drive key whose agent is not in the org — rejects
// the whole write.
func (b *relationBatch) validate(tx *gorm.DB, orgID uuid.UUID) error {
	if err := b.validateRelations(tx, orgID); err != nil {
		return err
	}
	return b.validateDriveOwners(tx, orgID)
}

func (b *relationBatch) validateRelations(tx *gorm.DB, orgID uuid.UUID) error {
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

// validateDriveOwners resolves every queued pub/e/{agentID}/… attachment key
// with one indexed lookup on the agents table: each agent ID must belong to
// the caller's org. Agents in another org (or non-existent agents) reject
// the whole write.
func (b *relationBatch) validateDriveOwners(tx *gorm.DB, orgID uuid.UUID) error {
	if len(b.driveKeys) == 0 {
		return nil
	}
	agentIDs := make([]uuid.UUID, 0, len(b.driveKeys))
	for id := range b.driveKeys {
		agentIDs = append(agentIDs, id)
	}
	sort.Slice(agentIDs, func(i, j int) bool { return agentIDs[i].String() < agentIDs[j].String() })
	var found []uuid.UUID
	err := tx.Model(&model.Agent{}).
		Where("id IN ? AND org_id = ?", agentIDs, orgID).
		Pluck("id", &found).Error
	if err != nil {
		return fmt.Errorf("check attachment drive owners: %w", err)
	}
	foundSet := make(map[uuid.UUID]bool, len(found))
	for _, id := range found {
		foundSet[id] = true
	}
	for _, id := range agentIDs {
		if !foundSet[id] {
			ref := b.driveKeys[id][0]
			return &AttachmentError{FieldID: ref.fieldID, Key: ref.key, Message: "object key is not owned by this org"}
		}
	}
	return nil
}

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
