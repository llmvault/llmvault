package handler

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sheets"
)

type sheetSummary struct {
	ID          string    `json:"id"`
	Slug        string    `json:"slug"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Icon        string    `json:"icon"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type sheetFieldView struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Type     string         `json:"type"`
	Options  map[string]any `json:"options"`
	Position float64        `json:"position"`
}

type sheetPageView struct {
	ID             string           `json:"id"`
	SheetID        string           `json:"sheet_id"`
	Name           string           `json:"name"`
	Position       float64          `json:"position"`
	DisplayFieldID *string          `json:"display_field_id,omitempty"`
	Fields         []sheetFieldView `json:"fields"`
	RowCount       int64            `json:"row_count"`
}

type sheetStructureResponse struct {
	Sheet sheetSummary    `json:"sheet"`
	Pages []sheetPageView `json:"pages"`
}

type sheetRowView struct {
	ID        string         `json:"id"`
	Data      map[string]any `json:"data"`
	Position  float64        `json:"position"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

type sheetViewSummary struct {
	ID       string         `json:"id"`
	PageID   string         `json:"page_id"`
	Name     string         `json:"name"`
	Type     string         `json:"type"`
	Config   map[string]any `json:"config"`
	Position float64        `json:"position"`
}

type sheetOperationView struct {
	ID              string     `json:"id"`
	PageID          string     `json:"page_id"`
	Type            string     `json:"type"`
	RowCount        int        `json:"row_count"`
	ActorAgentID    *uuid.UUID `json:"actor_agent_id,omitempty"`
	ActorUserID     *uuid.UUID `json:"actor_user_id,omitempty"`
	SourceSessionID *uuid.UUID `json:"source_session_id,omitempty"`
	RevertedAt      *time.Time `json:"reverted_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

type sheetImportJobView struct {
	ID            string    `json:"id"`
	PageID        string    `json:"page_id"`
	ObjectKey     string    `json:"object_key"`
	Status        string    `json:"status"`
	TotalRows     int64     `json:"total_rows"`
	ProcessedRows int64     `json:"processed_rows"`
	Error         string    `json:"error"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func sheetSummaryFrom(m model.Sheet) sheetSummary {
	return sheetSummary{
		ID: m.ID.String(), Slug: m.Slug, Name: m.Name,
		Description: m.Description, Icon: m.Icon,
		CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
}

func sheetFieldViewFrom(m model.SheetField) sheetFieldView {
	return sheetFieldView{
		ID: m.ID, Name: m.Name, Type: m.Type,
		Options: map[string]any(m.Options), Position: m.Position,
	}
}

func sheetPageViewFrom(p model.SheetPage, fields []model.SheetField, rowCount int64) sheetPageView {
	view := sheetPageView{
		ID: p.ID.String(), SheetID: p.SheetID.String(), Name: p.Name,
		Position: p.Position, DisplayFieldID: p.DisplayFieldID,
		Fields: make([]sheetFieldView, 0, len(fields)), RowCount: rowCount,
	}
	for _, field := range fields {
		view.Fields = append(view.Fields, sheetFieldViewFrom(field))
	}
	return view
}

func sheetRowViewFrom(m model.SheetRow) sheetRowView {
	return sheetRowView{
		ID: m.ID.String(), Data: map[string]any(m.Data), Position: m.Position,
		CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
}

func sheetRowViewsFrom(rows []model.SheetRow) []sheetRowView {
	out := make([]sheetRowView, 0, len(rows))
	for _, row := range rows {
		out = append(out, sheetRowViewFrom(row))
	}
	return out
}

func sheetViewSummaryFrom(m model.SheetView) sheetViewSummary {
	return sheetViewSummary{
		ID: m.ID.String(), PageID: m.PageID.String(), Name: m.Name,
		Type: m.Type, Config: map[string]any(m.Config), Position: m.Position,
	}
}

func sheetOperationViewFrom(m model.SheetOperation) sheetOperationView {
	return sheetOperationView{
		ID: m.ID.String(), PageID: m.PageID.String(), Type: m.Type,
		RowCount: m.RowCount, ActorAgentID: m.ActorAgentID,
		ActorUserID: m.ActorUserID, SourceSessionID: m.SourceSessionID,
		RevertedAt: m.RevertedAt, CreatedAt: m.CreatedAt,
	}
}

func sheetImportJobViewFrom(m model.SheetImportJob) sheetImportJobView {
	return sheetImportJobView{
		ID: m.ID.String(), PageID: m.PageID.String(), ObjectKey: m.ObjectKey,
		Status: m.Status, TotalRows: m.TotalRows, ProcessedRows: m.ProcessedRows,
		Error: m.Error, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
}

func (h *SheetsHandler) requireSheetsOrg(w http.ResponseWriter, r *http.Request) (*model.Org, bool) {
	if h == nil || h.svc == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "sheets are not configured"})
		return nil, false
	}
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok || org == nil {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing org context"})
		return nil, false
	}
	return org, true
}

// sheetsActor attributes mutations to the authenticated user when the
// route group resolved one (API-key callers have no user).
func sheetsActor(r *http.Request) sheets.Actor {
	actor := sheets.Actor{}
	if user, ok := middleware.UserFromContext(r.Context()); ok && user != nil {
		actor.UserID = &user.ID
	}
	return actor
}

func sheetsPathUUID(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, name))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: name + " must be a uuid"})
		return uuid.Nil, false
	}
	return id, true
}

// sheetsNestedPageID is the single choke point for every nested
// /sheets/{sheetID}/pages/{pageID}/... route: it parses both path IDs and
// enforces that the page belongs to the sheet, writing a 404 on mismatch so
// a same-org page can never be addressed under the wrong sheet.
func (h *SheetsHandler) sheetsNestedPageID(w http.ResponseWriter, r *http.Request, org *model.Org) (uuid.UUID, bool) {
	sheetID, ok := sheetsPathUUID(w, r, "sheetID")
	if !ok {
		return uuid.Nil, false
	}
	pageID, ok := sheetsPathUUID(w, r, "pageID")
	if !ok {
		return uuid.Nil, false
	}
	if err := h.svc.RequirePageInSheet(r.Context(), org.ID, sheetID, pageID); err != nil {
		writeSheetsError(w, r, err)
		return uuid.Nil, false
	}
	return pageID, true
}

// writeSheetsError maps service errors onto HTTP statuses: unknown scope →
// 404, limit breaches → 422, validation → 400, double revert → 409.
func writeSheetsError(w http.ResponseWriter, r *http.Request, err error) {
	var limitErr *sheets.LimitError
	switch {
	case errors.Is(err, sheets.ErrNotFound):
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
	case errors.Is(err, sheets.ErrAlreadyReverted):
		writeJSON(w, http.StatusConflict, errorResponse{Error: err.Error()})
	case errors.As(err, &limitErr):
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: err.Error()})
	case isSheetsBadRequest(err):
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
	default:
		logging.Capture(r.Context(), err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "sheets request failed"})
	}
}

func isSheetsBadRequest(err error) bool {
	var (
		unknownField *sheets.UnknownFieldError
		unknownType  *sheets.UnknownFieldTypeError
		badOp        *sheets.UnsupportedOpError
		badValue     *sheets.ValueError
		badOptions   *sheets.OptionsError
		badRelation  *sheets.RelationError
		badAttach    *sheets.AttachmentError
	)
	return errors.Is(err, sheets.ErrInvalidCursor) ||
		errors.Is(err, sheets.ErrCursorWithFieldSorts) ||
		errors.As(err, &unknownField) || errors.As(err, &unknownType) ||
		errors.As(err, &badOp) || errors.As(err, &badValue) ||
		errors.As(err, &badOptions) || errors.As(err, &badRelation) ||
		errors.As(err, &badAttach) ||
		// Remaining service-side validation errors share the "sheets: " prefix
		// (empty batches, filter shape, unknown view types).
		strings.HasPrefix(err.Error(), "sheets: ")
}
