package sheets

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/testdb"
)

type sheetsFixture struct {
	db           *gorm.DB
	svc          *Service
	org          model.Org
	otherOrg     model.Org
	user         model.User
	agent        model.Agent
	otherAgent   model.Agent
	channel      model.Channel
	otherChannel model.Channel
	session      model.Session
	actor        Actor

	sheet     *SheetStructure
	leads     PageStructure // page with one field per type (relation added below)
	companies PageStructure // relation target page

	otherSheet *SheetStructure
	otherPage  PageStructure
	otherRow   model.SheetRow
}

func connectSheetsTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(postgres.Open(testdb.DatabaseURL()), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect test db: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(3)
	sqlDB.SetMaxIdleConns(1)
	testdb.ApplyMigrations(t, db)
	t.Cleanup(func() { sqlDB.Close() })
	return db
}

func seedSheetsFixture(t *testing.T, db *gorm.DB) *sheetsFixture {
	t.Helper()
	ctx := context.Background()
	f := &sheetsFixture{db: db, svc: NewService(db)}
	f.org = model.Org{ID: uuid.New(), Name: "sheets-" + uuid.NewString(), Active: true, RateLimit: 1000}
	f.otherOrg = model.Org{ID: uuid.New(), Name: "sheets-other-" + uuid.NewString(), Active: true, RateLimit: 1000}
	f.user = model.User{ID: uuid.New(), Email: "sheets-" + uuid.NewString() + "@example.com", Name: "Sheets Tester"}
	f.agent = model.Agent{ID: uuid.New(), OrgID: &f.org.ID, Name: "Sheets Agent " + uuid.NewString(), Model: "test", Status: "active"}
	f.otherAgent = model.Agent{ID: uuid.New(), OrgID: &f.otherOrg.ID, Name: "Sheets Agent " + uuid.NewString(), Model: "test", Status: "active"}
	f.channel = model.Channel{ID: uuid.New(), OrgID: f.org.ID, Name: "sheets-ch-" + uuid.NewString(), DefaultAgentID: f.agent.ID}
	f.otherChannel = model.Channel{ID: uuid.New(), OrgID: f.otherOrg.ID, Name: "sheets-ch-" + uuid.NewString(), DefaultAgentID: f.otherAgent.ID}
	f.session = model.Session{ID: uuid.New(), OrgID: f.org.ID, ChannelID: f.channel.ID, AgentID: f.agent.ID}
	for _, seed := range []any{&f.org, &f.otherOrg, &f.user, &f.agent, &f.otherAgent, &f.channel, &f.otherChannel, &f.session} {
		if err := db.Create(seed).Error; err != nil {
			t.Fatalf("seed fixture record: %v", err)
		}
	}
	t.Cleanup(func() {
		db.Delete(&model.Org{}, "id IN ?", []uuid.UUID{f.org.ID, f.otherOrg.ID})
		db.Delete(&model.User{}, "id = ?", f.user.ID)
	})
	f.actor = Actor{AgentID: &f.agent.ID, ChannelID: f.channel.ID}

	sheet, err := f.svc.CreateSheet(ctx, f.org.ID, CreateSheetRequest{
		Name: "Leads " + uuid.NewString(),
		Pages: []PageSpec{
			{Name: "Leads", Fields: []FieldSpec{
				{Name: "Name", Type: FieldTypeText},
				{Name: "Notes", Type: FieldTypeLongText},
				{Name: "Score", Type: FieldTypeNumber},
				{Name: "Active", Type: FieldTypeCheckbox},
				{Name: "Status", Type: FieldTypeSelect, Options: model.JSON{"choices": []any{"new", "qualified"}}},
				{Name: "Tags", Type: FieldTypeMultiSelect, Options: model.JSON{"choices": []any{"a", "b", "c"}}},
				{Name: "When", Type: FieldTypeDate},
				{Name: "Site", Type: FieldTypeURL},
				{Name: "Mail", Type: FieldTypeEmail},
				{Name: "Phone", Type: FieldTypePhone},
				{Name: "Files", Type: FieldTypeAttachment},
			}},
			{Name: "Companies", Fields: []FieldSpec{
				{Name: "Company", Type: FieldTypeText},
			}},
		},
	}, f.actor)
	if err != nil {
		t.Fatalf("create fixture sheet: %v", err)
	}
	f.sheet = sheet
	f.leads = sheet.Pages[0]
	f.companies = sheet.Pages[1]

	other, err := f.svc.CreateSheet(ctx, f.otherOrg.ID, CreateSheetRequest{
		Name: "Other Org Sheet",
		Pages: []PageSpec{{Name: "Other", Fields: []FieldSpec{
			{Name: "Secret", Type: FieldTypeText},
		}}},
	}, Actor{ChannelID: f.otherChannel.ID})
	if err != nil {
		t.Fatalf("create other-org sheet: %v", err)
	}
	f.otherSheet = other
	f.otherPage = other.Pages[0]
	rows, err := f.svc.InsertRows(ctx, f.otherOrg.ID, f.otherPage.Page.ID, []RowInsert{
		{Data: map[string]any{f.otherPage.Fields[0].ID: "classified"}},
	}, MaxRowsPerWriteMCP, Actor{})
	if err != nil {
		t.Fatalf("insert other-org row: %v", err)
	}
	f.otherRow = rows[0]
	return f
}

func (f *sheetsFixture) fieldByName(t *testing.T, page PageStructure, name string) *model.SheetField {
	t.Helper()
	for i := range page.Fields {
		if page.Fields[i].Name == name {
			return &page.Fields[i]
		}
	}
	t.Fatalf("fixture page %s has no field named %q", page.Page.Name, name)
	return nil
}

// createRelationField adds a relation column on the leads page targeting the
// companies page via the real service path.
func (f *sheetsFixture) createRelationField(t *testing.T, targetPageID uuid.UUID) *model.SheetField {
	t.Helper()
	field, err := f.svc.CreateField(context.Background(), f.org.ID, f.leads.Page.ID, FieldSpec{
		Name:    "Linked " + uuid.NewString()[:8],
		Type:    FieldTypeRelation,
		Options: model.JSON{"target_page_id": targetPageID.String()},
	}, f.actor)
	if err != nil {
		t.Fatalf("create relation field: %v", err)
	}
	return field
}

func (f *sheetsFixture) insertLeads(t *testing.T, rows ...map[string]any) []model.SheetRow {
	t.Helper()
	inserts := make([]RowInsert, 0, len(rows))
	for _, data := range rows {
		inserts = append(inserts, RowInsert{Data: data})
	}
	out, err := f.svc.InsertRows(context.Background(), f.org.ID, f.leads.Page.ID, inserts, MaxRowsPerWriteMCP, f.actor)
	if err != nil {
		t.Fatalf("insert leads: %v", err)
	}
	return out
}

func (f *sheetsFixture) reloadRow(t *testing.T, id uuid.UUID) model.SheetRow {
	t.Helper()
	var row model.SheetRow
	if err := f.db.First(&row, "id = ?", id).Error; err != nil {
		t.Fatalf("reload row %s: %v", id, err)
	}
	return row
}

func (f *sheetsFixture) lastOperation(t *testing.T, pageID uuid.UUID) model.SheetOperation {
	t.Helper()
	ops, err := f.svc.ListOperations(context.Background(), f.org.ID, pageID, 1)
	if err != nil {
		t.Fatalf("list operations: %v", err)
	}
	if len(ops) == 0 {
		t.Fatalf("no operations recorded for page %s", pageID)
	}
	return ops[0]
}
