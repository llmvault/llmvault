package sheets

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

// RowInsert is one row to insert; Data is keyed by field ID.
type RowInsert struct {
	Data     map[string]any
	Position *float64
}

// RowUpdate is a partial update: only the keys present in Data are touched;
// a nil value clears that cell. Position moves the row (drag-reorder); it is
// not captured in the operation inverse — undo restores cells only.
type RowUpdate struct {
	ID       uuid.UUID
	Data     map[string]any
	Position *float64
}

// InsertRows validates, coerces, and inserts a batch of rows, recording one
// rows_insert operation. maxBatch is the caller-surface limit
// (MaxRowsPerWriteMCP or MaxRowsPerWriteREST).
func (s *Service) InsertRows(ctx context.Context, orgID, pageID uuid.UUID, inserts []RowInsert, maxBatch int, actor Actor) ([]model.SheetRow, error) {
	if len(inserts) == 0 {
		return nil, fmt.Errorf("sheets: no rows to insert")
	}
	if len(inserts) > maxBatch {
		return nil, &LimitError{Limit: "rows per write", Max: maxBatch, Actual: len(inserts)}
	}
	page, fields, err := s.loadPageContext(ctx, orgID, pageID)
	if err != nil {
		return nil, err
	}
	var existing int64
	if err := s.db.WithContext(ctx).Model(&model.SheetRow{}).
		Where("page_id = ? AND org_id = ? AND archived_at IS NULL", page.ID, orgID).
		Count(&existing).Error; err != nil {
		return nil, fmt.Errorf("count page rows: %w", err)
	}
	if int(existing)+len(inserts) > MaxRowsPerPage {
		return nil, &LimitError{Limit: "rows per page", Max: MaxRowsPerPage, Actual: int(existing) + len(inserts)}
	}

	batch := newRelationBatch()
	rows := make([]model.SheetRow, 0, len(inserts))
	basePosition, err := s.nextPosition(ctx, &model.SheetRow{}, "page_id = ? AND org_id = ?", page.ID, orgID)
	if err != nil {
		return nil, err
	}
	for i, insert := range inserts {
		data, err := coerceCells(orgID, fields, insert.Data, batch)
		if err != nil {
			return nil, err
		}
		data = stripNilCells(data)
		if err := checkRowSize(data); err != nil {
			return nil, err
		}
		position := basePosition + float64(i*positionStep)
		if insert.Position != nil {
			position = *insert.Position
		}
		rows = append(rows, model.SheetRow{
			PageID:           page.ID,
			OrgID:            orgID,
			Data:             model.JSON(data),
			Position:         position,
			CreatedByAgentID: actor.AgentID,
			CreatedByUserID:  actor.UserID,
		})
	}

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := batch.validate(tx, orgID); err != nil {
			return err
		}
		if err := tx.CreateInBatches(&rows, 500).Error; err != nil {
			return fmt.Errorf("insert sheet rows: %w", err)
		}
		rowIDs := make([]any, 0, len(rows))
		for _, row := range rows {
			rowIDs = append(rowIDs, row.ID.String())
		}
		return s.recordOperationTx(tx, &model.SheetOperation{
			OrgID:    orgID,
			PageID:   page.ID,
			Type:     OpTypeRowsInsert,
			RowCount: len(rows),
			Inverse:  model.JSON{"row_ids": rowIDs},
		}, actor)
	})
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(rows))
	patches := make(map[string]map[string]any, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID.String())
		patches[row.ID.String()] = map[string]any(row.Data)
	}
	s.publishRowsChanged(ctx, page, "insert", ids, patches, rows, actor)
	return rows, nil
}

// UpdateRows applies partial merges per field key to a batch of rows and
// records one rows_update operation whose inverse holds only the changed
// keys' prior values.
func (s *Service) UpdateRows(ctx context.Context, orgID, pageID uuid.UUID, updates []RowUpdate, maxBatch int, actor Actor) ([]model.SheetRow, error) {
	if len(updates) == 0 {
		return nil, fmt.Errorf("sheets: no rows to update")
	}
	if len(updates) > maxBatch {
		return nil, &LimitError{Limit: "rows per write", Max: maxBatch, Actual: len(updates)}
	}
	page, fields, err := s.loadPageContext(ctx, orgID, pageID)
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, 0, len(updates))
	seen := make(map[uuid.UUID]bool, len(updates))
	for _, update := range updates {
		if seen[update.ID] {
			return nil, fmt.Errorf("sheets: duplicate row id %s in update batch", update.ID)
		}
		seen[update.ID] = true
		ids = append(ids, update.ID)
	}

	var out []model.SheetRow
	forward := make(map[string]map[string]any, len(updates))
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rows []model.SheetRow
		if err := tx.
			Where("id IN ? AND page_id = ? AND org_id = ? AND archived_at IS NULL", ids, page.ID, orgID).
			Find(&rows).Error; err != nil {
			return fmt.Errorf("load sheet rows: %w", err)
		}
		byID := make(map[uuid.UUID]*model.SheetRow, len(rows))
		for i := range rows {
			byID[rows[i].ID] = &rows[i]
		}
		batch := newRelationBatch()
		patches := map[string]any{}
		for _, update := range updates {
			row, ok := byID[update.ID]
			if !ok {
				return ErrNotFound
			}
			incoming, err := coerceCells(orgID, fields, update.Data, batch)
			if err != nil {
				return err
			}
			merged, inverse := mergeCells(row.Data, incoming)
			if err := checkRowSize(merged); err != nil {
				return err
			}
			row.Data = model.JSON(merged)
			patches[row.ID.String()] = inverse
			forward[row.ID.String()] = incoming
		}
		if err := batch.validate(tx, orgID); err != nil {
			return err
		}
		now := time.Now().UTC()
		for _, update := range updates {
			row := byID[update.ID]
			changes := map[string]any{"data": row.Data, "updated_at": now}
			if update.Position != nil {
				changes["position"] = *update.Position
				row.Position = *update.Position
			}
			if err := tx.Model(&model.SheetRow{}).
				Where("id = ? AND org_id = ?", row.ID, orgID).
				Updates(changes).Error; err != nil {
				return fmt.Errorf("update sheet row: %w", err)
			}
			// Keep the returned snapshot's timestamp consistent with the row
			// the DB now holds (the live event carries these snapshots).
			row.UpdatedAt = now
			out = append(out, *row)
		}
		return s.recordOperationTx(tx, &model.SheetOperation{
			OrgID:    orgID,
			PageID:   page.ID,
			Type:     OpTypeRowsUpdate,
			RowCount: len(updates),
			Inverse:  model.JSON{"patches": patches},
		}, actor)
	})
	if err != nil {
		return nil, err
	}
	updatedIDs := make([]string, 0, len(out))
	for _, row := range out {
		updatedIDs = append(updatedIDs, row.ID.String())
	}
	s.publishRowsChanged(ctx, page, "update", updatedIDs, forward, out, actor)
	return out, nil
}

// DeleteRows soft-archives a batch of rows and records one rows_delete
// operation; undo simply un-archives.
func (s *Service) DeleteRows(ctx context.Context, orgID, pageID uuid.UUID, ids []uuid.UUID, maxBatch int, actor Actor) (int, error) {
	if len(ids) == 0 {
		return 0, fmt.Errorf("sheets: no rows to delete")
	}
	if len(ids) > maxBatch {
		return 0, &LimitError{Limit: "rows per write", Max: maxBatch, Actual: len(ids)}
	}
	page, err := s.loadPage(ctx, orgID, pageID)
	if err != nil {
		return 0, err
	}
	archived := 0
	var matched []string
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.SheetRow{}).
			Where("id IN ? AND page_id = ? AND org_id = ? AND archived_at IS NULL", ids, page.ID, orgID).
			Pluck("id", &matched).Error; err != nil {
			return fmt.Errorf("match sheet rows: %w", err)
		}
		if len(matched) == 0 {
			return ErrNotFound
		}
		if err := tx.Model(&model.SheetRow{}).
			Where("id IN ? AND page_id = ? AND org_id = ?", matched, page.ID, orgID).
			Update("archived_at", time.Now().UTC()).Error; err != nil {
			return fmt.Errorf("archive sheet rows: %w", err)
		}
		archived = len(matched)
		rowIDs := make([]any, 0, len(matched))
		for _, id := range matched {
			rowIDs = append(rowIDs, id)
		}
		return s.recordOperationTx(tx, &model.SheetOperation{
			OrgID:    orgID,
			PageID:   page.ID,
			Type:     OpTypeRowsDelete,
			RowCount: len(matched),
			Inverse:  model.JSON{"row_ids": rowIDs},
		}, actor)
	})
	if err != nil {
		return 0, err
	}
	s.publishRowsChanged(ctx, page, "delete", matched, nil, nil, actor)
	return archived, nil
}

// loadPageContext loads a page and its active fields, both org-scoped.
func (s *Service) loadPageContext(ctx context.Context, orgID, pageID uuid.UUID) (*model.SheetPage, map[string]*model.SheetField, error) {
	page, err := s.loadPage(ctx, orgID, pageID)
	if err != nil {
		return nil, nil, err
	}
	fields, err := s.loadPageFields(ctx, orgID, page.ID)
	if err != nil {
		return nil, nil, err
	}
	return page, FieldMap(fields), nil
}
