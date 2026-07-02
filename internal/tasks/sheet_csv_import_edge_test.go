package tasks

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sheets"
	"github.com/usehivy/hivy/internal/testdb"
)

func TestSheetCSVImportRowLimitFailsFast(t *testing.T) {
	f := newSheetImportFixture(t, nil)
	body := strings.Repeat("x\n", sheets.MaxRowsPerPage+1)
	job := f.createJob(t, body, model.JSON{"has_header": false})

	if err := f.handle(t, job.ID); err != nil {
		t.Fatalf("limit violation is terminal, got %v", err)
	}
	reloaded := f.reloadJob(t, job.ID)
	if reloaded.Status != sheets.ImportStatusFailed || !strings.Contains(reloaded.Error, "rows per page") {
		t.Fatalf("job = status %q error %q, want failed on row cap", reloaded.Status, reloaded.Error)
	}
	if got := len(f.importedRows(t, job.ID)); got != 0 {
		t.Fatalf("rows inserted despite fail-fast: %d", got)
	}
	if got := len(f.pageFields(t)); got != 0 {
		t.Fatalf("fields created despite fail-fast: %d", got)
	}
}

func TestSheetCSVImportTerminalCoercionFailureRollsBack(t *testing.T) {
	f := newSheetImportFixture(t, leadFieldSpecs())
	csvBody := "score\n10\nnot-a-number\n"
	job := f.createJob(t, csvBody, model.JSON{
		"field_mapping": map[string]any{"score": f.fieldID(t, "Score")},
	})

	if err := f.handle(t, job.ID); err != nil {
		t.Fatalf("coercion failure is terminal, got %v", err)
	}
	reloaded := f.reloadJob(t, job.ID)
	if reloaded.Status != sheets.ImportStatusFailed || !strings.Contains(reloaded.Error, "not a number") {
		t.Fatalf("job = status %q error %q", reloaded.Status, reloaded.Error)
	}
	if got := len(f.importedRows(t, job.ID)); got != 0 {
		t.Fatalf("rows survived rollback: %d", got)
	}
	if reloaded.ProcessedRows != 0 {
		t.Fatalf("processed_rows = %d after rollback, want 0", reloaded.ProcessedRows)
	}
}

func TestSheetCSVImportOptionErrorsAreTerminal(t *testing.T) {
	cases := []struct {
		name    string
		options model.JSON
		errPart string
	}{
		{"bad delimiter", model.JSON{"delimiter": "ab"}, "delimiter"},
		{"nothing to do", model.JSON{"create_fields": false}, "field_mapping or create_fields"},
		{"mapping misses columns", model.JSON{"field_mapping": map[string]any{"nope": "fld_00000000000000000000000000"}}, "field_mapping"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newSheetImportFixture(t, leadFieldSpecs())
			options := tc.options
			if tc.name == "mapping misses columns" {
				options = model.JSON{"field_mapping": map[string]any{"nope": f.fieldID(t, "Name")}}
			}
			job := f.createJob(t, "a,b\n1,2\n", options)
			if err := f.handle(t, job.ID); err != nil {
				t.Fatalf("option errors are terminal, got %v", err)
			}
			reloaded := f.reloadJob(t, job.ID)
			if reloaded.Status != sheets.ImportStatusFailed || !strings.Contains(reloaded.Error, tc.errPart) {
				t.Fatalf("job = status %q error %q, want failed containing %q", reloaded.Status, reloaded.Error, tc.errPart)
			}
		})
	}
}

func TestSheetCSVImportEnforcesJobOrgOnPage(t *testing.T) {
	f := newSheetImportFixture(t, nil)
	other := newSheetImportFixture(t, nil)

	// A forged job pointing at another org's page must fail through the
	// service's org scoping, writing nothing to the foreign page.
	forged := model.SheetImportJob{
		OrgID:     f.org.ID,
		PageID:    other.page.Page.ID,
		ObjectKey: f.objectKey(),
		Status:    sheets.ImportStatusPending,
		Options:   model.JSON{},
	}
	f.reader.put(f.objectKey(), "name\nAcme\n")
	if err := f.db.Create(&forged).Error; err != nil {
		t.Fatalf("seed forged job: %v", err)
	}
	if err := f.handle(t, forged.ID); err != nil {
		t.Fatalf("cross-org page is terminal, got %v", err)
	}
	reloaded := f.reloadJob(t, forged.ID)
	if reloaded.Status != sheets.ImportStatusFailed {
		t.Fatalf("forged job status = %q, want failed", reloaded.Status)
	}
	var count int64
	f.db.Model(&model.SheetRow{}).Where("page_id = ?", other.page.Page.ID).Count(&count)
	if count != 0 {
		t.Fatalf("forged job wrote %d rows into another org's page", count)
	}
}

func TestSheetCSVImportInferenceRules(t *testing.T) {
	cases := []struct {
		name   string
		values []string
		want   string
	}{
		{"zeros and ones are numbers", []string{"0", "1", "0"}, sheets.FieldTypeNumber},
		{"explicit booleans", []string{"true", "no", "YES"}, sheets.FieldTypeCheckbox},
		{"floats", []string{"3.14", "2", "-1.5"}, sheets.FieldTypeNumber},
		{"dates", []string{"2024-01-01", "2024-06-30T12:00:00Z"}, sheets.FieldTypeDate},
		{"emails", []string{"a@b.com", "c@d.io"}, sheets.FieldTypeEmail},
		{"urls", []string{"https://a.com", "http://b.io/x"}, sheets.FieldTypeURL},
		{"mixed falls back to text", []string{"2024-01-01", "hello"}, sheets.FieldTypeText},
		{"numbers then text falls back", []string{"12", "hello"}, sheets.FieldTypeText},
		{"empty column is text", []string{"", "  "}, sheets.FieldTypeText},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sample := make([][]string, 0, len(tc.values))
			for _, v := range tc.values {
				sample = append(sample, []string{v})
			}
			if got := inferColumnType(sample, 0); got != tc.want {
				t.Fatalf("inferColumnType(%v) = %q, want %q", tc.values, got, tc.want)
			}
		})
	}
}

func TestSheetImportEnqueuerAdapter(t *testing.T) {
	enq := &fakeTaskEnqueuer{}
	adapter := NewSheetImportEnqueuer(enq)
	jobID := model.SheetImportJob{}.ID
	if err := adapter.EnqueueSheetCSVImport(context.Background(), jobID); err == nil {
		t.Fatalf("nil job id must be rejected")
	}

	f := newSheetImportFixture(t, leadFieldSpecs())
	job := f.createJob(t, "name\nAcme\n", model.JSON{})
	if err := adapter.EnqueueSheetCSVImport(context.Background(), job.ID); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if len(enq.tasks) != 1 || enq.tasks[0].Type() != TypeSheetCSVImport {
		t.Fatalf("enqueued tasks = %+v", enq.tasks)
	}
	var payload SheetCSVImportPayload
	if err := json.Unmarshal(enq.tasks[0].Payload(), &payload); err != nil || payload.JobID != job.ID {
		t.Fatalf("payload = %+v err=%v", payload, err)
	}
	if len(enq.opts[0]) != 3 {
		t.Fatalf("enqueue opts = %d, want queue+retry+timeout returned separately", len(enq.opts[0]))
	}

	var nilAdapter *SheetImportEnqueuer
	if err := nilAdapter.EnqueueSheetCSVImport(context.Background(), job.ID); err == nil {
		t.Fatalf("nil adapter must error so the service marks the job failed")
	}
}

func TestSheetCSVImportPublishesProgressToRedis(t *testing.T) {
	rc := redis.NewClient(&redis.Options{Addr: testdb.RedisAddr("HIVY_REDIS_ADDR", "TEST_REDIS_ADDR")})
	t.Cleanup(func() { rc.Close() })
	if err := rc.Ping(context.Background()).Err(); err != nil {
		t.Skipf("Redis is not available: %v", err)
	}

	f := newSheetImportFixture(t, leadFieldSpecs())
	f.handler = NewSheetCSVImportHandler(f.db, f.reader, sheets.NewRedisEventPublisher(rc))
	job := f.createJob(t, "name\nAcme\nGlobex\n", model.JSON{
		"field_mapping": map[string]any{"name": f.fieldID(t, "Name")},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	sub := rc.Subscribe(ctx, sheets.LiveChannel(f.sheet.Sheet.ID))
	t.Cleanup(func() { sub.Close() })
	if _, err := sub.Receive(ctx); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	if err := f.handle(t, job.ID); err != nil {
		t.Fatalf("handle: %v", err)
	}

	ch := sub.Channel()
	for {
		select {
		case msg := <-ch:
			var event sheets.Event
			if err := json.Unmarshal([]byte(msg.Payload), &event); err != nil {
				t.Fatalf("decode event: %v", err)
			}
			if event.Type == sheets.EventImportProgress && event.JobID == job.ID.String() &&
				event.Status == sheets.ImportStatusCompleted && event.ProcessedRows == 2 {
				return
			}
		case <-ctx.Done():
			t.Fatalf("no completed import_progress event on %s", sheets.LiveChannel(f.sheet.Sheet.ID))
		}
	}
}
