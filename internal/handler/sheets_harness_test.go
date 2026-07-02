package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sheets"
)

const sheetsTestSigningKey = "sheets-test-signing-key"

type sheetsHarness struct {
	db      *gorm.DB
	svc     *sheets.Service
	handler *handler.SheetsHandler
	router  *chi.Mux
	org     model.Org
	other   model.Org
	user    model.User

	sheet  *sheets.SheetStructure
	page   sheets.PageStructure
	fields map[string]string // field name -> id

	otherSheet *sheets.SheetStructure
	otherPage  sheets.PageStructure
}

type fakeSheetsPresigner struct{}

func (fakeSheetsPresigner) PresignGet(_ context.Context, key string) (string, error) {
	return "https://storage.test/" + key, nil
}

func newSheetsHarness(t *testing.T) *sheetsHarness {
	t.Helper()
	db := connectTestDB(t)
	h := &sheetsHarness{db: db, svc: sheets.NewService(db)}
	h.org = createTestOrg(t, db)
	h.other = createTestOrg(t, db)
	h.user = model.User{ID: uuid.New(), Email: "sheets-" + uuid.NewString() + "@example.com", Name: "Sheets User"}
	if err := db.Create(&h.user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	t.Cleanup(func() {
		db.Delete(&model.Sheet{}, "org_id IN ?", []uuid.UUID{h.org.ID, h.other.ID})
		db.Delete(&model.User{}, "id = ?", h.user.ID)
	})

	ctx := context.Background()
	sheet, err := h.svc.CreateSheet(ctx, h.org.ID, sheets.CreateSheetRequest{
		Name: "Leads",
		Pages: []sheets.PageSpec{{Name: "Leads", Fields: []sheets.FieldSpec{
			{Name: "Name", Type: sheets.FieldTypeText},
			{Name: "Score", Type: sheets.FieldTypeNumber},
			{Name: "Mail", Type: sheets.FieldTypeEmail},
		}}},
	}, sheets.Actor{UserID: &h.user.ID})
	if err != nil {
		t.Fatalf("create sheet: %v", err)
	}
	h.sheet = sheet
	h.page = sheet.Pages[0]
	h.fields = map[string]string{}
	for _, field := range h.page.Fields {
		h.fields[field.Name] = field.ID
	}

	other, err := h.svc.CreateSheet(ctx, h.other.ID, sheets.CreateSheetRequest{
		Name: "Other",
		Pages: []sheets.PageSpec{{Name: "Secrets", Fields: []sheets.FieldSpec{
			{Name: "Secret", Type: sheets.FieldTypeText},
		}}},
	}, sheets.Actor{})
	if err != nil {
		t.Fatalf("create other sheet: %v", err)
	}
	h.otherSheet = other
	h.otherPage = other.Pages[0]

	h.handler = handler.NewSheetsHandler(db, h.svc, []byte(sheetsTestSigningKey)).
		WithPresigner(fakeSheetsPresigner{})
	h.router = newSheetsRouter(h.handler)
	return h
}

// newSheetsRouter mirrors production registration: the live SSE route sits at
// the router root (outside /v1 auth) while everything else lives under the
// mounted /v1 group — also guarding chi's mount/backtrack behavior.
func newSheetsRouter(sheetsHandler *handler.SheetsHandler) *chi.Mux {
	router := chi.NewRouter()
	router.Get("/v1/sheets/{sheetID}/live", sheetsHandler.Live)
	router.Route("/v1", func(r chi.Router) {
		r.Route("/sheets", func(r chi.Router) {
			r.Get("/", sheetsHandler.ListSheets)
			r.Post("/", sheetsHandler.CreateSheet)
			r.Get("/imports/{jobID}", sheetsHandler.GetImportJob)
			r.Route("/{sheetID}", func(r chi.Router) {
				r.Get("/", sheetsHandler.GetSheet)
				r.Patch("/", sheetsHandler.UpdateSheet)
				r.Delete("/", sheetsHandler.ArchiveSheet)
				r.Post("/live-token", sheetsHandler.LiveToken)
				r.Post("/pages", sheetsHandler.CreatePage)
				r.Route("/pages/{pageID}", func(r chi.Router) {
					r.Patch("/", sheetsHandler.UpdatePage)
					r.Delete("/", sheetsHandler.ArchivePage)
					r.Post("/fields", sheetsHandler.CreateField)
					r.Patch("/fields/{fieldID}", sheetsHandler.UpdateField)
					r.Delete("/fields/{fieldID}", sheetsHandler.ArchiveField)
					r.Post("/rows/query", sheetsHandler.QueryRows)
					r.Post("/rows", sheetsHandler.InsertRows)
					r.Patch("/rows", sheetsHandler.UpdateRows)
					r.Delete("/rows", sheetsHandler.DeleteRows)
					r.Get("/views", sheetsHandler.ListViews)
					r.Post("/views", sheetsHandler.CreateView)
					r.Patch("/views/{viewID}", sheetsHandler.UpdateView)
					r.Delete("/views/{viewID}", sheetsHandler.ArchiveView)
					r.Post("/attachments/download-url", sheetsHandler.AttachmentDownloadURL)
					r.Post("/imports", sheetsHandler.CreateImport)
					r.Get("/export.csv", sheetsHandler.ExportCSV)
					r.Get("/operations", sheetsHandler.ListOperations)
					r.Post("/operations/{operationID}/revert", sheetsHandler.RevertOperation)
				})
			})
		})
	})
	return router
}

func (h *sheetsHarness) do(t *testing.T, org *model.Org, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if org != nil {
		req = middleware.WithOrg(req, org)
		req = middleware.WithUser(req, &h.user)
	}
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

// createAgent seeds an active agent in orgID for drive-key ownership tests.
// Rows are cascade-deleted with the org (createTestOrg cleanup).
func (h *sheetsHarness) createAgent(t *testing.T, orgID uuid.UUID) model.Agent {
	t.Helper()
	agent := model.Agent{ID: uuid.New(), OrgID: &orgID, Name: "Sheets Agent " + uuid.NewString(), Model: "test", Status: "active"}
	if err := h.db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	t.Cleanup(func() { h.db.Delete(&model.Agent{}, "id = ?", agent.ID) })
	return agent
}

func (h *sheetsHarness) pagePath(suffix string) string {
	return "/v1/sheets/" + h.sheet.Sheet.ID.String() + "/pages/" + h.page.Page.ID.String() + suffix
}

func (h *sheetsHarness) insertRows(t *testing.T, rows ...map[string]any) []string {
	t.Helper()
	payload := make([]map[string]any, 0, len(rows))
	for _, data := range rows {
		payload = append(payload, map[string]any{"data": data})
	}
	resp := h.do(t, &h.org, http.MethodPost, h.pagePath("/rows"), map[string]any{"rows": payload})
	if resp.Code != http.StatusCreated {
		t.Fatalf("insert rows status=%d body=%s", resp.Code, resp.Body.String())
	}
	var decoded struct {
		Rows []struct {
			ID string `json:"id"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("decode insert response: %v", err)
	}
	ids := make([]string, 0, len(decoded.Rows))
	for _, row := range decoded.Rows {
		ids = append(ids, row.ID)
	}
	return ids
}
