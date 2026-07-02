package sheets

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

func TestOperationInverseRoundTrips(t *testing.T) {
	ctx := context.Background()
	db := connectSheetsTestDB(t)
	f := seedSheetsFixture(t, db)
	nameID := f.fieldByName(t, f.leads, "Name").ID
	scoreID := f.fieldByName(t, f.leads, "Score").ID

	// rows_insert → revert archives the inserted rows.
	rows := f.insertLeads(t, map[string]any{nameID: "A"}, map[string]any{nameID: "B"})
	op := f.lastOperation(t, f.leads.Page.ID)
	if op.Type != OpTypeRowsInsert || op.RowCount != 2 {
		t.Fatalf("insert op = %s count %d", op.Type, op.RowCount)
	}
	if err := f.svc.RevertOperation(ctx, f.org.ID, op.ID, f.actor); err != nil {
		t.Fatalf("revert insert: %v", err)
	}
	for _, row := range rows {
		if f.reloadRow(t, row.ID).ArchivedAt == nil {
			t.Fatalf("insert revert should archive row %s", row.ID)
		}
	}
	// Reverting twice fails.
	if err := f.svc.RevertOperation(ctx, f.org.ID, op.ID, f.actor); !errors.Is(err, ErrAlreadyReverted) {
		t.Fatalf("double revert error = %v, want ErrAlreadyReverted", err)
	}
	// Cross-org revert is invisible.
	if err := f.svc.RevertOperation(ctx, f.otherOrg.ID, op.ID, Actor{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-org revert error = %v, want ErrNotFound", err)
	}

	// rows_update → revert patches back only the changed keys.
	rows = f.insertLeads(t, map[string]any{nameID: "Original"})
	if _, err := f.svc.UpdateRows(ctx, f.org.ID, f.leads.Page.ID, []RowUpdate{
		{ID: rows[0].ID, Data: map[string]any{nameID: "Changed", scoreID: 42}},
	}, MaxRowsPerWriteMCP, f.actor); err != nil {
		t.Fatalf("update rows: %v", err)
	}
	op = f.lastOperation(t, f.leads.Page.ID)
	if op.Type != OpTypeRowsUpdate {
		t.Fatalf("update op type = %s", op.Type)
	}
	if err := f.svc.RevertOperation(ctx, f.org.ID, op.ID, f.actor); err != nil {
		t.Fatalf("revert update: %v", err)
	}
	reverted := f.reloadRow(t, rows[0].ID)
	if reverted.Data[nameID] != "Original" {
		t.Fatalf("update revert should restore prior value, got %#v", reverted.Data[nameID])
	}
	if _, exists := reverted.Data[scoreID]; exists {
		t.Fatalf("update revert should remove keys that did not exist before: %#v", reverted.Data)
	}

	// rows_delete → revert un-archives (data was never lost).
	if _, err := f.svc.DeleteRows(ctx, f.org.ID, f.leads.Page.ID, []uuid.UUID{rows[0].ID}, MaxRowsPerWriteMCP, f.actor); err != nil {
		t.Fatalf("delete rows: %v", err)
	}
	op = f.lastOperation(t, f.leads.Page.ID)
	if op.Type != OpTypeRowsDelete {
		t.Fatalf("delete op type = %s", op.Type)
	}
	if err := f.svc.RevertOperation(ctx, f.org.ID, op.ID, f.actor); err != nil {
		t.Fatalf("revert delete: %v", err)
	}
	if f.reloadRow(t, rows[0].ID).ArchivedAt != nil {
		t.Fatalf("delete revert should un-archive the row")
	}
	if op = f.lastOperation(t, f.leads.Page.ID); op.RevertedAt == nil {
		t.Fatalf("reverted operation should be marked with reverted_at")
	}
}

func TestFieldChangeAndImportReverts(t *testing.T) {
	ctx := context.Background()
	db := connectSheetsTestDB(t)
	f := seedSheetsFixture(t, db)

	// add_field → revert archives the new column.
	field, err := f.svc.CreateField(ctx, f.org.ID, f.leads.Page.ID, FieldSpec{Name: "Extra", Type: FieldTypeText}, f.actor)
	if err != nil {
		t.Fatalf("create field: %v", err)
	}
	op := f.lastOperation(t, f.leads.Page.ID)
	if err := f.svc.RevertOperation(ctx, f.org.ID, op.ID, f.actor); err != nil {
		t.Fatalf("revert create field: %v", err)
	}
	reloadField := func(id string) model.SheetField {
		t.Helper()
		var out model.SheetField
		if err := db.First(&out, "id = ?", id).Error; err != nil {
			t.Fatalf("reload field %s: %v", id, err)
		}
		return out
	}
	if reloadField(field.ID).ArchivedAt == nil {
		t.Fatalf("create-field revert should archive the field")
	}

	// update_field → revert restores the prior definition.
	statusField := f.fieldByName(t, f.leads, "Status")
	newName := "Stage"
	if _, err := f.svc.UpdateField(ctx, f.org.ID, statusField.ID, UpdateFieldRequest{Name: &newName}, f.actor); err != nil {
		t.Fatalf("update field: %v", err)
	}
	op = f.lastOperation(t, f.leads.Page.ID)
	if err := f.svc.RevertOperation(ctx, f.org.ID, op.ID, f.actor); err != nil {
		t.Fatalf("revert update field: %v", err)
	}
	if got := reloadField(statusField.ID); got.Name != "Status" {
		t.Fatalf("field revert should restore name, got %q", got.Name)
	}

	// archive_field → revert restores the column.
	if err := f.svc.ArchiveField(ctx, f.org.ID, statusField.ID, f.actor); err != nil {
		t.Fatalf("archive field: %v", err)
	}
	op = f.lastOperation(t, f.leads.Page.ID)
	if err := f.svc.RevertOperation(ctx, f.org.ID, op.ID, f.actor); err != nil {
		t.Fatalf("revert archive field: %v", err)
	}
	if got := reloadField(statusField.ID); got.ArchivedAt != nil {
		t.Fatalf("archive-field revert should restore the field")
	}

	// csv_import → revert archives everything stamped with the job id.
	job := model.SheetImportJob{OrgID: f.org.ID, PageID: f.leads.Page.ID, ObjectKey: "pub/o/x/import.csv", Status: "completed"}
	if err := db.Create(&job).Error; err != nil {
		t.Fatalf("create import job: %v", err)
	}
	nameID := f.fieldByName(t, f.leads, "Name").ID
	imported := model.SheetRow{PageID: f.leads.Page.ID, OrgID: f.org.ID, Data: model.JSON{nameID: "Imported"}, Position: 1, ImportJobID: &job.ID}
	if err := db.Create(&imported).Error; err != nil {
		t.Fatalf("create imported row: %v", err)
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		return f.svc.recordOperationTx(tx, &model.SheetOperation{
			OrgID: f.org.ID, PageID: f.leads.Page.ID, Type: OpTypeCSVImport,
			RowCount: 1, Inverse: model.JSON{"import_job_id": job.ID.String()},
		}, f.actor)
	})
	if err != nil {
		t.Fatalf("record import op: %v", err)
	}
	op = f.lastOperation(t, f.leads.Page.ID)
	if err := f.svc.RevertOperation(ctx, f.org.ID, op.ID, f.actor); err != nil {
		t.Fatalf("revert import: %v", err)
	}
	if f.reloadRow(t, imported.ID).ArchivedAt == nil {
		t.Fatalf("import revert should archive imported rows")
	}
}

func TestOperationPruneAtRetentionCap(t *testing.T) {
	ctx := context.Background()
	db := connectSheetsTestDB(t)
	f := seedSheetsFixture(t, db)
	nameID := f.fieldByName(t, f.leads, "Name").ID

	total := operationRetentionPerPage + 5
	for i := 0; i < total; i++ {
		f.insertLeads(t, map[string]any{nameID: fmt.Sprintf("row-%03d", i)})
	}
	var count int64
	if err := db.Model(&model.SheetOperation{}).
		Where("page_id = ? AND org_id = ?", f.leads.Page.ID, f.org.ID).
		Count(&count).Error; err != nil {
		t.Fatalf("count operations: %v", err)
	}
	if count != operationRetentionPerPage {
		t.Fatalf("operations after prune = %d, want %d", count, operationRetentionPerPage)
	}
	// The newest operations survive; the oldest were pruned.
	ops, err := f.svc.ListOperations(ctx, f.org.ID, f.leads.Page.ID, operationRetentionPerPage)
	if err != nil {
		t.Fatalf("list operations: %v", err)
	}
	if len(ops) != operationRetentionPerPage {
		t.Fatalf("listed %d operations, want %d", len(ops), operationRetentionPerPage)
	}
	for i := 1; i < len(ops); i++ {
		if ops[i].CreatedAt.After(ops[i-1].CreatedAt) {
			t.Fatalf("operations not sorted newest-first")
		}
	}
	// Other org's page log is untouched by pruning (scoped per page+org).
	otherOps, err := f.svc.ListOperations(ctx, f.otherOrg.ID, f.otherPage.Page.ID, 10)
	if err != nil {
		t.Fatalf("list other org operations: %v", err)
	}
	if len(otherOps) != 1 {
		t.Fatalf("other org should keep its single insert op, got %d", len(otherOps))
	}
}
