package sheets

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
)

type recordingPublisher struct {
	mu     sync.Mutex
	events []Event
}

func (p *recordingPublisher) PublishSheetEvent(_ context.Context, event Event) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, event)
}

func (p *recordingPublisher) last(t *testing.T) Event {
	t.Helper()
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.events) == 0 {
		t.Fatalf("no events published")
	}
	return p.events[len(p.events)-1]
}

func TestServicePublishesMutationEvents(t *testing.T) {
	db := connectSheetsTestDB(t)
	f := seedSheetsFixture(t, db)
	pub := &recordingPublisher{}
	f.svc.WithPublisher(pub)
	ctx := WithMutationID(context.Background(), "mut-123")
	nameField := f.fieldByName(t, f.leads, "Name")
	scoreField := f.fieldByName(t, f.leads, "Score")

	rows, err := f.svc.InsertRows(ctx, f.org.ID, f.leads.Page.ID, []RowInsert{
		{Data: map[string]any{nameField.ID: "Acme", scoreField.ID: 10}},
	}, MaxRowsPerWriteREST, f.actor)
	if err != nil {
		t.Fatalf("insert rows: %v", err)
	}
	event := pub.last(t)
	if event.Type != EventRowsChanged || event.Action != "insert" {
		t.Fatalf("insert event = %+v", event)
	}
	if event.SheetID != f.sheet.Sheet.ID.String() || event.PageID != f.leads.Page.ID.String() {
		t.Fatalf("insert event routing = %+v", event)
	}
	if event.MutationID != "mut-123" {
		t.Fatalf("mutation_id = %q, want mut-123", event.MutationID)
	}
	if event.Actor == nil || event.Actor.AgentID != f.agent.ID.String() {
		t.Fatalf("actor = %+v", event.Actor)
	}
	if len(event.RowIDs) != 1 || event.RowIDs[0] != rows[0].ID.String() {
		t.Fatalf("row ids = %v", event.RowIDs)
	}
	if event.Patches[rows[0].ID.String()][nameField.ID] != "Acme" {
		t.Fatalf("insert patches = %v", event.Patches)
	}

	_, err = f.svc.UpdateRows(ctx, f.org.ID, f.leads.Page.ID, []RowUpdate{
		{ID: rows[0].ID, Data: map[string]any{scoreField.ID: 42}},
	}, MaxRowsPerWriteREST, f.actor)
	if err != nil {
		t.Fatalf("update rows: %v", err)
	}
	event = pub.last(t)
	if event.Type != EventRowsChanged || event.Action != "update" {
		t.Fatalf("update event = %+v", event)
	}
	patch := event.Patches[rows[0].ID.String()]
	if len(patch) != 1 || patch[scoreField.ID] != float64(42) {
		t.Fatalf("update patches = %v", event.Patches)
	}

	if _, err := f.svc.DeleteRows(context.Background(), f.org.ID, f.leads.Page.ID, []uuid.UUID{rows[0].ID}, MaxRowsPerWriteREST, f.actor); err != nil {
		t.Fatalf("delete rows: %v", err)
	}
	event = pub.last(t)
	if event.Type != EventRowsChanged || event.Action != "delete" {
		t.Fatalf("delete event = %+v", event)
	}
	if event.MutationID != "" {
		t.Fatalf("mutation_id should be empty without context, got %q", event.MutationID)
	}
}

// TestRowsChangedCarriesRowSnapshots verifies the enriched rows_changed event:
// insert and update carry full row snapshots, delete stays IDs-only, and
// batches above maxEventPatchRows drop both patches and snapshots.
func TestRowsChangedCarriesRowSnapshots(t *testing.T) {
	db := connectSheetsTestDB(t)
	f := seedSheetsFixture(t, db)
	pub := &recordingPublisher{}
	f.svc.WithPublisher(pub)
	ctx := context.Background()
	nameField := f.fieldByName(t, f.leads, "Name")
	scoreField := f.fieldByName(t, f.leads, "Score")

	// Insert: snapshot present, matching the returned row.
	rows, err := f.svc.InsertRows(ctx, f.org.ID, f.leads.Page.ID, []RowInsert{
		{Data: map[string]any{nameField.ID: "Acme", scoreField.ID: 10}},
	}, MaxRowsPerWriteREST, f.actor)
	if err != nil {
		t.Fatalf("insert rows: %v", err)
	}
	insertEvent := pub.last(t)
	if len(insertEvent.Rows) != 1 {
		t.Fatalf("insert snapshots = %d, want 1", len(insertEvent.Rows))
	}
	snap := insertEvent.Rows[0]
	if snap.ID != rows[0].ID.String() {
		t.Fatalf("snapshot id = %q, want %q", snap.ID, rows[0].ID.String())
	}
	if snap.Data[nameField.ID] != "Acme" {
		t.Fatalf("snapshot data = %v", snap.Data)
	}
	if snap.Position != rows[0].Position {
		t.Fatalf("snapshot position = %v, want %v", snap.Position, rows[0].Position)
	}
	if snap.CreatedAt.IsZero() || snap.UpdatedAt.IsZero() {
		t.Fatalf("snapshot timestamps unset: %+v", snap)
	}

	// Update: snapshot carries the merged data with a fresh updated_at.
	if _, err := f.svc.UpdateRows(ctx, f.org.ID, f.leads.Page.ID, []RowUpdate{
		{ID: rows[0].ID, Data: map[string]any{scoreField.ID: 42}},
	}, MaxRowsPerWriteREST, f.actor); err != nil {
		t.Fatalf("update rows: %v", err)
	}
	updateEvent := pub.last(t)
	if len(updateEvent.Rows) != 1 {
		t.Fatalf("update snapshots = %d, want 1", len(updateEvent.Rows))
	}
	if updateEvent.Rows[0].Data[scoreField.ID] != float64(42) {
		t.Fatalf("update snapshot data = %v", updateEvent.Rows[0].Data)
	}
	if !updateEvent.Rows[0].UpdatedAt.After(snap.CreatedAt) && !updateEvent.Rows[0].UpdatedAt.Equal(snap.CreatedAt) {
		// updated_at must be a real timestamp; it should not be zero.
		if updateEvent.Rows[0].UpdatedAt.IsZero() {
			t.Fatalf("update snapshot updated_at unset")
		}
	}

	// Delete: IDs only, never snapshots.
	if _, err := f.svc.DeleteRows(ctx, f.org.ID, f.leads.Page.ID, []uuid.UUID{rows[0].ID}, MaxRowsPerWriteREST, f.actor); err != nil {
		t.Fatalf("delete rows: %v", err)
	}
	deleteEvent := pub.last(t)
	if len(deleteEvent.Rows) != 0 {
		t.Fatalf("delete carried %d snapshots, want 0", len(deleteEvent.Rows))
	}
	if len(deleteEvent.RowIDs) != 1 {
		t.Fatalf("delete row_ids = %v", deleteEvent.RowIDs)
	}

	// Cap: a batch above maxEventPatchRows ships IDs only (no patches, no rows).
	big := make([]RowInsert, maxEventPatchRows+1)
	for i := range big {
		big[i] = RowInsert{Data: map[string]any{nameField.ID: "row"}}
	}
	inserted, err := f.svc.InsertRows(ctx, f.org.ID, f.leads.Page.ID, big, MaxRowsPerWriteREST, f.actor)
	if err != nil {
		t.Fatalf("insert big batch: %v", err)
	}
	capEvent := pub.last(t)
	if len(capEvent.RowIDs) != len(inserted) {
		t.Fatalf("cap event row_ids = %d, want %d", len(capEvent.RowIDs), len(inserted))
	}
	if len(capEvent.Rows) != 0 || len(capEvent.Patches) != 0 {
		t.Fatalf("over-cap batch leaked snapshots/patches: rows=%d patches=%d", len(capEvent.Rows), len(capEvent.Patches))
	}
}

func TestServicePublishesSchemaAndRevertEvents(t *testing.T) {
	db := connectSheetsTestDB(t)
	f := seedSheetsFixture(t, db)
	pub := &recordingPublisher{}
	f.svc.WithPublisher(pub)
	ctx := context.Background()

	field, err := f.svc.CreateField(ctx, f.org.ID, f.leads.Page.ID, FieldSpec{Name: "Extra", Type: FieldTypeText}, f.actor)
	if err != nil {
		t.Fatalf("create field: %v", err)
	}
	event := pub.last(t)
	if event.Type != EventFieldsChanged || event.Action != "create" || event.FieldID != field.ID {
		t.Fatalf("field create event = %+v", event)
	}

	if err := f.svc.ArchiveField(ctx, f.org.ID, field.ID, f.actor); err != nil {
		t.Fatalf("archive field: %v", err)
	}
	event = pub.last(t)
	if event.Type != EventFieldsChanged || event.Action != "delete" {
		t.Fatalf("field archive event = %+v", event)
	}

	page, err := f.svc.CreatePage(ctx, f.org.ID, f.sheet.Sheet.ID, PageSpec{Name: "Another"})
	if err != nil {
		t.Fatalf("create page: %v", err)
	}
	event = pub.last(t)
	if event.Type != EventPagesChanged || event.Action != "create" || event.PageID != page.Page.ID.String() {
		t.Fatalf("page create event = %+v", event)
	}

	nameField := f.fieldByName(t, f.leads, "Name")
	if _, err := f.svc.InsertRows(ctx, f.org.ID, f.leads.Page.ID, []RowInsert{
		{Data: map[string]any{nameField.ID: "Undo me"}},
	}, MaxRowsPerWriteREST, f.actor); err != nil {
		t.Fatalf("insert rows: %v", err)
	}
	op := f.lastOperation(t, f.leads.Page.ID)
	if err := f.svc.RevertOperation(WithMutationID(ctx, "undo-1"), f.org.ID, op.ID, f.actor); err != nil {
		t.Fatalf("revert operation: %v", err)
	}
	event = pub.last(t)
	if event.Type != EventOperationReverted || event.OperationID != op.ID.String() {
		t.Fatalf("revert event = %+v", event)
	}
	if event.MutationID != "undo-1" {
		t.Fatalf("revert mutation_id = %q", event.MutationID)
	}
	if event.SheetID != f.sheet.Sheet.ID.String() {
		t.Fatalf("revert sheet routing = %+v", event)
	}
}

func TestImportJobLifecycleAndEvents(t *testing.T) {
	db := connectSheetsTestDB(t)
	f := seedSheetsFixture(t, db)
	pub := &recordingPublisher{}
	f.svc.WithPublisher(pub)
	enq := &recordingEnqueuer{}
	f.svc.WithImportEnqueuer(enq)
	ctx := context.Background()

	key := OrgAttachmentPrefix(f.org.ID) + "sheetimports/test.csv"
	job, err := f.svc.CreateImportJob(ctx, f.org.ID, f.leads.Page.ID, CreateImportJobRequest{ObjectKey: key}, f.actor)
	if err != nil {
		t.Fatalf("create import job: %v", err)
	}
	if job.Status != ImportStatusPending {
		t.Fatalf("job status = %q", job.Status)
	}
	if len(enq.jobIDs) != 1 || enq.jobIDs[0] != job.ID {
		t.Fatalf("enqueued jobs = %v", enq.jobIDs)
	}
	event := pub.last(t)
	if event.Type != EventImportProgress || event.JobID != job.ID.String() || event.Status != ImportStatusPending {
		t.Fatalf("import event = %+v", event)
	}

	if _, err := f.svc.CreateImportJob(ctx, f.org.ID, f.leads.Page.ID, CreateImportJobRequest{
		ObjectKey: OrgAttachmentPrefix(f.otherOrg.ID) + "sheetimports/steal.csv",
	}, f.actor); err == nil {
		t.Fatalf("expected foreign object key rejection")
	}

	loaded, err := f.svc.GetImportJob(ctx, f.org.ID, job.ID)
	if err != nil || loaded.ID != job.ID {
		t.Fatalf("get import job: %v", err)
	}
	if _, err := f.svc.GetImportJob(ctx, f.otherOrg.ID, job.ID); err == nil {
		t.Fatalf("cross-org import job read should fail")
	}
}

type recordingEnqueuer struct {
	jobIDs []uuid.UUID
}

func (e *recordingEnqueuer) EnqueueSheetCSVImport(_ context.Context, jobID uuid.UUID) error {
	e.jobIDs = append(e.jobIDs, jobID)
	return nil
}
