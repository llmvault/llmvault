package sheets

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/model"
)

// Operation types. Each records an inverse payload sufficient to undo
// exactly that one operation.
const (
	OpTypeRowsInsert  = "rows_insert"
	OpTypeRowsUpdate  = "rows_update"
	OpTypeRowsDelete  = "rows_delete"
	OpTypeCSVImport   = "csv_import"
	OpTypeFieldChange = "field_change"
)

// recordOperationTx writes one operation row and, inside the same
// transaction, prunes the page's log beyond the newest
// operationRetentionPerPage entries. Pruning only discards undo inverses,
// never data.
func (s *Service) recordOperationTx(tx *gorm.DB, op *model.SheetOperation, actor Actor) error {
	op.ActorAgentID = actor.AgentID
	op.ActorUserID = actor.UserID
	op.SourceSessionID = actor.SessionID
	if op.Inverse == nil {
		op.Inverse = model.JSON{}
	}
	if err := tx.Create(op).Error; err != nil {
		return fmt.Errorf("record sheet operation: %w", err)
	}
	err := tx.Exec(`
		DELETE FROM sheet_operations
		WHERE page_id = ? AND org_id = ?
		  AND id NOT IN (
			SELECT id FROM sheet_operations
			WHERE page_id = ? AND org_id = ?
			ORDER BY created_at DESC, id DESC
			LIMIT ?
		  )`,
		op.PageID, op.OrgID, op.PageID, op.OrgID, operationRetentionPerPage,
	).Error
	if err != nil {
		return fmt.Errorf("prune sheet operations: %w", err)
	}
	return nil
}

// ListOperations returns a page's recent operations, newest first.
func (s *Service) ListOperations(ctx context.Context, orgID, pageID uuid.UUID, limit int) ([]model.SheetOperation, error) {
	var ops []model.SheetOperation
	err := s.db.WithContext(ctx).
		Where("page_id = ? AND org_id = ?", pageID, orgID).
		Order("created_at DESC, id DESC").
		Limit(ClampLimit(limit, operationRetentionPerPage)).
		Find(&ops).Error
	if err != nil {
		return nil, fmt.Errorf("list sheet operations: %w", err)
	}
	return ops, nil
}

// RevertOperation applies an operation's inverse in one transaction and
// marks it reverted. Reverts are single-level: a revert is not itself
// re-undoable in v1.
func (s *Service) RevertOperation(ctx context.Context, orgID, operationID uuid.UUID, actor Actor) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var op model.SheetOperation
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND org_id = ?", operationID, orgID).
			First(&op).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("load sheet operation: %w", err)
		}
		if op.RevertedAt != nil {
			return ErrAlreadyReverted
		}
		if err := s.applyInverse(tx, &op); err != nil {
			return err
		}
		updates := map[string]any{
			"reverted_at":          time.Now().UTC(),
			"reverted_by_agent_id": actor.AgentID,
			"reverted_by_user_id":  actor.UserID,
		}
		if err := tx.Model(&model.SheetOperation{}).
			Where("id = ? AND org_id = ?", op.ID, orgID).
			Updates(updates).Error; err != nil {
			return fmt.Errorf("mark operation reverted: %w", err)
		}
		return nil
	})
}

func (s *Service) applyInverse(tx *gorm.DB, op *model.SheetOperation) error {
	switch op.Type {
	case OpTypeRowsInsert:
		return s.archiveInverseRows(tx, op, inverseRowIDs(op.Inverse))
	case OpTypeRowsDelete:
		ids := inverseRowIDs(op.Inverse)
		if len(ids) == 0 {
			return nil
		}
		err := tx.Model(&model.SheetRow{}).
			Where("id IN ? AND page_id = ? AND org_id = ?", ids, op.PageID, op.OrgID).
			Update("archived_at", nil).Error
		if err != nil {
			return fmt.Errorf("unarchive rows: %w", err)
		}
		return nil
	case OpTypeRowsUpdate:
		return s.revertRowPatches(tx, op)
	case OpTypeCSVImport:
		jobID, _ := op.Inverse["import_job_id"].(string)
		if jobID == "" {
			return fmt.Errorf("sheets: csv_import inverse missing import_job_id")
		}
		err := tx.Model(&model.SheetRow{}).
			Where("import_job_id = ? AND page_id = ? AND org_id = ? AND archived_at IS NULL", jobID, op.PageID, op.OrgID).
			Update("archived_at", time.Now().UTC()).Error
		if err != nil {
			return fmt.Errorf("archive imported rows: %w", err)
		}
		return nil
	case OpTypeFieldChange:
		return s.revertFieldChange(tx, op)
	default:
		return fmt.Errorf("sheets: cannot revert operation type %q", op.Type)
	}
}

func (s *Service) archiveInverseRows(tx *gorm.DB, op *model.SheetOperation, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	err := tx.Model(&model.SheetRow{}).
		Where("id IN ? AND page_id = ? AND org_id = ? AND archived_at IS NULL", ids, op.PageID, op.OrgID).
		Update("archived_at", time.Now().UTC()).Error
	if err != nil {
		return fmt.Errorf("archive inserted rows: %w", err)
	}
	return nil
}

// revertRowPatches restores only the changed keys' prior values. A nil prior
// value means the key did not exist before the update and is removed.
func (s *Service) revertRowPatches(tx *gorm.DB, op *model.SheetOperation) error {
	patches, _ := op.Inverse["patches"].(map[string]any)
	for rowIDRaw, patchRaw := range patches {
		rowID, err := uuid.Parse(rowIDRaw)
		if err != nil {
			return fmt.Errorf("sheets: invalid row id in inverse patch: %q", rowIDRaw)
		}
		patch, _ := patchRaw.(map[string]any)
		var row model.SheetRow
		err = tx.Where("id = ? AND page_id = ? AND org_id = ?", rowID, op.PageID, op.OrgID).
			First(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			continue
		}
		if err != nil {
			return fmt.Errorf("load row for revert: %w", err)
		}
		data := map[string]any(row.Data)
		if data == nil {
			data = map[string]any{}
		}
		for key, prior := range patch {
			if prior == nil {
				delete(data, key)
				continue
			}
			data[key] = prior
		}
		if err := tx.Model(&model.SheetRow{}).
			Where("id = ? AND org_id = ?", row.ID, op.OrgID).
			Update("data", model.JSON(data)).Error; err != nil {
			return fmt.Errorf("revert row patch: %w", err)
		}
	}
	return nil
}

func (s *Service) revertFieldChange(tx *gorm.DB, op *model.SheetOperation) error {
	fieldID, _ := op.Inverse["field_id"].(string)
	action, _ := op.Inverse["action"].(string)
	if !ValidFieldID(fieldID) {
		return fmt.Errorf("sheets: field_change inverse has invalid field id %q", fieldID)
	}
	switch action {
	case "archive":
		err := tx.Model(&model.SheetField{}).
			Where("id = ? AND page_id = ? AND org_id = ? AND archived_at IS NULL", fieldID, op.PageID, op.OrgID).
			Update("archived_at", time.Now().UTC()).Error
		if err != nil {
			return fmt.Errorf("archive field: %w", err)
		}
		return nil
	case "restore":
		prior, _ := op.Inverse["prior"].(map[string]any)
		if prior == nil {
			return fmt.Errorf("sheets: field_change inverse missing prior definition")
		}
		updates := map[string]any{"archived_at": nil}
		if name, ok := prior["name"].(string); ok && name != "" {
			updates["name"] = name
		}
		if fieldType, ok := prior["type"].(string); ok && KnownFieldType(fieldType) {
			updates["type"] = fieldType
		}
		if options, ok := prior["options"].(map[string]any); ok {
			updates["options"] = model.JSON(options)
		}
		if position, ok := prior["position"].(float64); ok {
			updates["position"] = position
		}
		err := tx.Model(&model.SheetField{}).
			Where("id = ? AND page_id = ? AND org_id = ?", fieldID, op.PageID, op.OrgID).
			Updates(updates).Error
		if err != nil {
			return fmt.Errorf("restore field: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("sheets: field_change inverse has unknown action %q", action)
	}
}

func inverseRowIDs(inverse model.JSON) []string {
	raw, _ := inverse["row_ids"].([]any)
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if s, ok := item.(string); ok {
			if _, err := uuid.Parse(s); err == nil {
				out = append(out, s)
			}
		}
	}
	return out
}
