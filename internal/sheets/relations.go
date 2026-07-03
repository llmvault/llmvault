package sheets

import (
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
