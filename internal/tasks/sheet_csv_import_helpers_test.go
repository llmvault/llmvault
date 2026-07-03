package tasks

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sheets"
)

// fakeObjectReader is an in-memory storage.Reader.
type fakeObjectReader struct {
	mu      sync.Mutex
	objects map[string][]byte
	opens   int
}

func (f *fakeObjectReader) Open(_ context.Context, key string) (io.ReadCloser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.opens++
	raw, ok := f.objects[key]
	if !ok {
		return nil, fmt.Errorf("fake storage: no object %q", key)
	}
	return io.NopCloser(bytes.NewReader(raw)), nil
}

func (f *fakeObjectReader) put(key string, body string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.objects == nil {
		f.objects = map[string][]byte{}
	}
	f.objects[key] = []byte(body)
}

// capturingSheetPublisher records sheet events in-process.
type capturingSheetPublisher struct {
	mu     sync.Mutex
	events []sheets.Event
}

func (p *capturingSheetPublisher) PublishSheetEvent(_ context.Context, event sheets.Event) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, event)
}

func (p *capturingSheetPublisher) importEvents() []sheets.Event {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]sheets.Event, 0, len(p.events))
	for _, event := range p.events {
		if event.Type == sheets.EventImportProgress {
			out = append(out, event)
		}
	}
	return out
}

type sheetImportFixture struct {
	db      *gorm.DB
	svc     *sheets.Service
	org     model.Org
	user    model.User
	sheet   *sheets.SheetStructure
	page    sheets.PageStructure
	reader  *fakeObjectReader
	pub     *capturingSheetPublisher
	handler *SheetCSVImportHandler
}

// newSheetImportFixture seeds an org with one sheet whose first page carries
// the given field specs (empty = the create_fields inference path).
func newSheetImportFixture(t *testing.T, fields []sheets.FieldSpec) *sheetImportFixture {
	t.Helper()
	db := connectTestDB(t)
	f := &sheetImportFixture{
		db:     db,
		svc:    sheets.NewService(db),
		reader: &fakeObjectReader{},
		pub:    &capturingSheetPublisher{},
	}
	f.org = model.Org{ID: uuid.New(), Name: "csvimport-" + uuid.NewString(), Active: true, RateLimit: 1000}
	f.user = model.User{ID: uuid.New(), Email: "csvimport-" + uuid.NewString() + "@example.com", Name: "Importer"}
	agent := model.Agent{ID: uuid.New(), OrgID: &f.org.ID, Name: "Import Agent " + uuid.NewString(), Model: "test", Status: "active"}
	channel := model.Channel{ID: uuid.New(), OrgID: f.org.ID, Name: "csvimport-ch-" + uuid.NewString(), DefaultAgentID: agent.ID}
	for _, seed := range []any{&f.org, &f.user, &agent, &channel} {
		if err := db.Create(seed).Error; err != nil {
			t.Fatalf("seed fixture record: %v", err)
		}
	}
	t.Cleanup(func() {
		db.Delete(&model.Org{}, "id = ?", f.org.ID)
		db.Delete(&model.User{}, "id = ?", f.user.ID)
	})

	sheet, err := f.svc.CreateSheet(context.Background(), f.org.ID, sheets.CreateSheetRequest{
		Name:  "Import " + uuid.NewString(),
		Pages: []sheets.PageSpec{{Name: "Data", Fields: fields}},
	}, sheets.Actor{UserID: &f.user.ID, ChannelID: channel.ID})
	if err != nil {
		t.Fatalf("create fixture sheet: %v", err)
	}
	f.sheet = sheet
	f.page = sheet.Pages[0]
	f.handler = NewSheetCSVImportHandler(db, f.reader, f.pub)
	return f
}

// leadFieldSpecs is the mapped-import fixture page: one text, number, and
// checkbox column.
func leadFieldSpecs() []sheets.FieldSpec {
	return []sheets.FieldSpec{
		{Name: "Name", Type: sheets.FieldTypeText},
		{Name: "Score", Type: sheets.FieldTypeNumber},
		{Name: "Active", Type: sheets.FieldTypeCheckbox},
	}
}

func (f *sheetImportFixture) fieldID(t *testing.T, name string) string {
	t.Helper()
	for _, field := range f.page.Fields {
		if field.Name == name {
			return field.ID
		}
	}
	t.Fatalf("fixture page has no field %q", name)
	return ""
}

func (f *sheetImportFixture) objectKey() string {
	return sheets.OrgAttachmentPrefix(f.org.ID) + "sheetimports/import.csv"
}

// createJob persists an import job through the service (no enqueuer set, so
// it stays pending for the handler to pick up).
func (f *sheetImportFixture) createJob(t *testing.T, csvBody string, options model.JSON) *model.SheetImportJob {
	t.Helper()
	f.reader.put(f.objectKey(), csvBody)
	job, err := f.svc.CreateImportJob(context.Background(), f.org.ID, f.page.Page.ID, sheets.CreateImportJobRequest{
		ObjectKey: f.objectKey(),
		Options:   options,
	}, sheets.Actor{UserID: &f.user.ID})
	if err != nil {
		t.Fatalf("create import job: %v", err)
	}
	return job
}

func (f *sheetImportFixture) handle(t *testing.T, jobID uuid.UUID) error {
	t.Helper()
	task, _, err := NewSheetCSVImportTask(SheetCSVImportPayload{JobID: jobID})
	if err != nil {
		t.Fatalf("build import task: %v", err)
	}
	return f.handler.Handle(context.Background(), task)
}

func (f *sheetImportFixture) reloadJob(t *testing.T, jobID uuid.UUID) model.SheetImportJob {
	t.Helper()
	var job model.SheetImportJob
	if err := f.db.First(&job, "id = ?", jobID).Error; err != nil {
		t.Fatalf("reload import job: %v", err)
	}
	return job
}

func (f *sheetImportFixture) importedRows(t *testing.T, jobID uuid.UUID) []model.SheetRow {
	t.Helper()
	var rows []model.SheetRow
	if err := f.db.Where("import_job_id = ?", jobID).Order("position ASC").Find(&rows).Error; err != nil {
		t.Fatalf("load imported rows: %v", err)
	}
	return rows
}

func (f *sheetImportFixture) pageFields(t *testing.T) []model.SheetField {
	t.Helper()
	fields, err := f.svc.ImportPageFields(context.Background(), f.org.ID, f.page.Page.ID)
	if err != nil {
		t.Fatalf("load page fields: %v", err)
	}
	return fields
}

func (f *sheetImportFixture) lastOperation(t *testing.T) model.SheetOperation {
	t.Helper()
	ops, err := f.svc.ListOperations(context.Background(), f.org.ID, f.page.Page.ID, 1)
	if err != nil {
		t.Fatalf("list operations: %v", err)
	}
	if len(ops) == 0 {
		t.Fatalf("no operations recorded")
	}
	return ops[0]
}

// fakeTaskEnqueuer captures enqueued tasks for adapter tests.
type fakeTaskEnqueuer struct {
	tasks []*asynq.Task
	opts  [][]asynq.Option
	err   error
}

func (e *fakeTaskEnqueuer) Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	return e.EnqueueContext(context.Background(), task, opts...)
}

func (e *fakeTaskEnqueuer) EnqueueContext(_ context.Context, task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	if e.err != nil {
		return nil, e.err
	}
	e.tasks = append(e.tasks, task)
	e.opts = append(e.opts, opts)
	return &asynq.TaskInfo{ID: uuid.NewString(), Type: task.Type()}, nil
}

func (e *fakeTaskEnqueuer) Close() error { return nil }
