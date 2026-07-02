package tasks

import (
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sheets"
)

func TestSheetCSVImportWithFieldMapping(t *testing.T) {
	f := newSheetImportFixture(t, leadFieldSpecs())
	csvBody := "name,score,active\nAcme,42,yes\nGlobex,7.5,no\n"
	job := f.createJob(t, csvBody, model.JSON{
		"has_header": true,
		"field_mapping": map[string]any{
			"name":   f.fieldID(t, "Name"),
			"score":  f.fieldID(t, "Score"),
			"active": f.fieldID(t, "Active"),
		},
	})

	if err := f.handle(t, job.ID); err != nil {
		t.Fatalf("handle: %v", err)
	}

	reloaded := f.reloadJob(t, job.ID)
	if reloaded.Status != sheets.ImportStatusCompleted {
		t.Fatalf("status = %q (error=%q), want completed", reloaded.Status, reloaded.Error)
	}
	if reloaded.TotalRows != 2 || reloaded.ProcessedRows != 2 {
		t.Fatalf("total/processed = %d/%d, want 2/2", reloaded.TotalRows, reloaded.ProcessedRows)
	}

	rows := f.importedRows(t, job.ID)
	if len(rows) != 2 {
		t.Fatalf("imported rows = %d, want 2", len(rows))
	}
	first := rows[0].Data
	if first[f.fieldID(t, "Name")] != "Acme" {
		t.Fatalf("name cell = %v", first[f.fieldID(t, "Name")])
	}
	if first[f.fieldID(t, "Score")] != float64(42) {
		t.Fatalf("score cell = %v (%T), want coerced number", first[f.fieldID(t, "Score")], first[f.fieldID(t, "Score")])
	}
	if first[f.fieldID(t, "Active")] != true {
		t.Fatalf("active cell = %v, want coerced bool", first[f.fieldID(t, "Active")])
	}
	if rows[1].Data[f.fieldID(t, "Active")] != false {
		t.Fatalf("second active cell = %v, want false", rows[1].Data[f.fieldID(t, "Active")])
	}

	op := f.lastOperation(t)
	if op.Type != sheets.OpTypeCSVImport || op.RowCount != 2 {
		t.Fatalf("operation = %+v, want csv_import x2", op)
	}
	if op.Inverse["import_job_id"] != job.ID.String() {
		t.Fatalf("operation inverse = %v", op.Inverse)
	}

	events := f.pub.importEvents()
	if len(events) < 2 {
		t.Fatalf("import_progress events = %d, want at least running+completed", len(events))
	}
	last := events[len(events)-1]
	if last.Status != sheets.ImportStatusCompleted || last.ProcessedRows != 2 || last.TotalRows != 2 || last.JobID != job.ID.String() {
		t.Fatalf("final progress event = %+v", last)
	}
	if last.SheetID != f.sheet.Sheet.ID.String() {
		t.Fatalf("progress event routed to sheet %q, want %q", last.SheetID, f.sheet.Sheet.ID)
	}
}

func TestSheetCSVImportHeaderlessIndexMappingAndDelimiter(t *testing.T) {
	f := newSheetImportFixture(t, leadFieldSpecs())
	csvBody := "Acme;42\nGlobex;7\n"
	job := f.createJob(t, csvBody, model.JSON{
		"has_header": false,
		"delimiter":  ";",
		"field_mapping": map[string]any{
			"0": f.fieldID(t, "Name"),
			"1": f.fieldID(t, "Score"),
		},
	})

	if err := f.handle(t, job.ID); err != nil {
		t.Fatalf("handle: %v", err)
	}
	reloaded := f.reloadJob(t, job.ID)
	if reloaded.Status != sheets.ImportStatusCompleted || reloaded.ProcessedRows != 2 {
		t.Fatalf("job = %+v", reloaded)
	}
	rows := f.importedRows(t, job.ID)
	if len(rows) != 2 || rows[1].Data[f.fieldID(t, "Score")] != float64(7) {
		t.Fatalf("rows = %+v", rows)
	}
}

func TestSheetCSVImportCreateFieldsInference(t *testing.T) {
	f := newSheetImportFixture(t, nil)
	csvBody := "Name,Score,Active,When,Mail,Site,Mixed\n" +
		"Acme,42,yes,2024-01-02,a@b.com,https://x.com,12\n" +
		"Globex,7.5,no,2024-02-03,c@d.io,https://y.io,hello\n"
	job := f.createJob(t, csvBody, model.JSON{})

	if err := f.handle(t, job.ID); err != nil {
		t.Fatalf("handle: %v", err)
	}
	reloaded := f.reloadJob(t, job.ID)
	if reloaded.Status != sheets.ImportStatusCompleted {
		t.Fatalf("status = %q (error=%q)", reloaded.Status, reloaded.Error)
	}

	want := map[string]string{
		"Name":   sheets.FieldTypeText,
		"Score":  sheets.FieldTypeNumber,
		"Active": sheets.FieldTypeCheckbox,
		"When":   sheets.FieldTypeDate,
		"Mail":   sheets.FieldTypeEmail,
		"Site":   sheets.FieldTypeURL,
		"Mixed":  sheets.FieldTypeText, // number then text → fallback
	}
	fields := f.pageFields(t)
	if len(fields) != len(want) {
		t.Fatalf("created fields = %d, want %d", len(fields), len(want))
	}
	byName := map[string]model.SheetField{}
	for _, field := range fields {
		byName[field.Name] = field
	}
	for name, wantType := range want {
		field, ok := byName[name]
		if !ok || field.Type != wantType {
			t.Fatalf("field %q = %+v, want type %q", name, field, wantType)
		}
	}

	rows := f.importedRows(t, job.ID)
	if len(rows) != 2 {
		t.Fatalf("imported rows = %d, want 2", len(rows))
	}
	if got := rows[0].Data[byName["When"].ID]; got != "2024-01-02T00:00:00Z" {
		t.Fatalf("date cell = %v, want normalized RFC3339", got)
	}
	if got := rows[1].Data[byName["Score"].ID]; got != float64(7.5) {
		t.Fatalf("score cell = %v", got)
	}
	if got := rows[0].Data[byName["Mixed"].ID]; got != "12" {
		t.Fatalf("mixed cell = %v, want raw text", got)
	}
}

func TestSheetCSVImportChunksAndProgress(t *testing.T) {
	f := newSheetImportFixture(t, nil)
	var body strings.Builder
	body.WriteString("Amount\n")
	rowCount := importChunkSize*2 + 101 // 3 chunks
	for i := 0; i < rowCount; i++ {
		fmt.Fprintf(&body, "%d\n", i)
	}
	job := f.createJob(t, body.String(), model.JSON{"create_fields": true})

	if err := f.handle(t, job.ID); err != nil {
		t.Fatalf("handle: %v", err)
	}
	reloaded := f.reloadJob(t, job.ID)
	if reloaded.Status != sheets.ImportStatusCompleted || reloaded.ProcessedRows != int64(rowCount) || reloaded.TotalRows != int64(rowCount) {
		t.Fatalf("job = status %q processed %d total %d", reloaded.Status, reloaded.ProcessedRows, reloaded.TotalRows)
	}
	if got := len(f.importedRows(t, job.ID)); got != rowCount {
		t.Fatalf("imported rows = %d, want %d", got, rowCount)
	}

	// Per-chunk progress: 500, 1000, 1101 must all have been published.
	seen := map[int64]bool{}
	for _, event := range f.pub.importEvents() {
		seen[event.ProcessedRows] = true
	}
	for _, milestone := range []int64{int64(importChunkSize), int64(2 * importChunkSize), int64(rowCount)} {
		if !seen[milestone] {
			t.Fatalf("no import_progress event at processed_rows=%d (seen %v)", milestone, seen)
		}
	}
}

func TestSheetCSVImportRetryWipesPartialRows(t *testing.T) {
	f := newSheetImportFixture(t, leadFieldSpecs())
	csvBody := "name\nAcme\nGlobex\n"
	job := f.createJob(t, csvBody, model.JSON{
		"field_mapping": map[string]any{"name": f.fieldID(t, "Name")},
	})

	// Simulate a prior partial attempt: stale stamped rows + running status.
	for i := 0; i < 3; i++ {
		stale := model.SheetRow{
			PageID:      f.page.Page.ID,
			OrgID:       f.org.ID,
			Data:        model.JSON{f.fieldID(t, "Name"): fmt.Sprintf("stale-%d", i)},
			Position:    float64(i + 1),
			ImportJobID: &job.ID,
		}
		if err := f.db.Create(&stale).Error; err != nil {
			t.Fatalf("seed stale row: %v", err)
		}
	}
	err := f.db.Model(&model.SheetImportJob{}).Where("id = ?", job.ID).
		Updates(map[string]any{"status": sheets.ImportStatusRunning, "processed_rows": 3}).Error
	if err != nil {
		t.Fatalf("mark job running: %v", err)
	}

	if err := f.handle(t, job.ID); err != nil {
		t.Fatalf("handle: %v", err)
	}
	reloaded := f.reloadJob(t, job.ID)
	if reloaded.Status != sheets.ImportStatusCompleted || reloaded.ProcessedRows != 2 {
		t.Fatalf("job = status %q processed %d, want completed/2", reloaded.Status, reloaded.ProcessedRows)
	}
	rows := f.importedRows(t, job.ID)
	if len(rows) != 2 {
		t.Fatalf("rows after retry = %d, want 2 (stale rows wiped)", len(rows))
	}
	for _, row := range rows {
		if name, _ := row.Data[f.fieldID(t, "Name")].(string); strings.HasPrefix(name, "stale-") {
			t.Fatalf("stale row survived retry: %v", row.Data)
		}
	}
}

func TestSheetCSVImportGoneJobReturnsNil(t *testing.T) {
	f := newSheetImportFixture(t, nil)
	if err := f.handle(t, uuid.New()); err != nil {
		t.Fatalf("gone job should not retry, got %v", err)
	}
}

func TestSheetCSVImportStaleStatusIsNoop(t *testing.T) {
	f := newSheetImportFixture(t, leadFieldSpecs())
	job := f.createJob(t, "name\nAcme\n", model.JSON{
		"field_mapping": map[string]any{"name": f.fieldID(t, "Name")},
	})
	if err := f.handle(t, job.ID); err != nil {
		t.Fatalf("first run: %v", err)
	}
	rowsBefore := len(f.importedRows(t, job.ID))

	// A duplicate delivery after completion must not touch the rows.
	if err := f.handle(t, job.ID); err != nil {
		t.Fatalf("stale delivery: %v", err)
	}
	if got := len(f.importedRows(t, job.ID)); got != rowsBefore {
		t.Fatalf("stale delivery changed rows: %d -> %d", rowsBefore, got)
	}
}
