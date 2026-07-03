package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/sheets"
)

// Internal app API row surface (apps plan §1.2). Request/response shapes are
// the sheets REST shapes (same package types), but the sheet is always the
// app's bound sheet and write batches share the 100-row MCP cap.

// SheetStructure handles GET /internal/apps/{appID}/v1/sheet — the bound
// sheet's pages, fields, and row counts. Read-only: structure never mutates
// through the app surface.
func (h *AppsInternalHandler) SheetStructure(w http.ResponseWriter, r *http.Request) {
	app, ok := h.authApp(w, r)
	if !ok {
		return
	}
	structure, err := h.svc.GetSheet(r.Context(), app.OrgID, app.SheetID)
	if err != nil {
		writeSheetsError(w, r, err)
		return
	}
	response := sheetStructureResponse{
		Sheet: sheetSummaryFrom(structure.Sheet),
		Pages: make([]sheetPageView, 0, len(structure.Pages)),
	}
	pageIDs := make([]uuid.UUID, 0, len(structure.Pages))
	for _, page := range structure.Pages {
		pageIDs = append(pageIDs, page.Page.ID)
	}
	counts, err := h.svc.PageRowCounts(r.Context(), app.OrgID, pageIDs)
	if err != nil {
		writeSheetsError(w, r, err)
		return
	}
	for _, page := range structure.Pages {
		response.Pages = append(response.Pages, sheetPageViewFrom(page.Page, page.Fields, counts[page.Page.ID]))
	}
	writeJSON(w, http.StatusOK, response)
}

// QueryRows handles POST /internal/apps/{appID}/v1/pages/{pageID}/rows/query.
func (h *AppsInternalHandler) QueryRows(w http.ResponseWriter, r *http.Request) {
	app, ok := h.authApp(w, r)
	if !ok {
		return
	}
	pageID, ok := h.appPageID(w, r, app)
	if !ok {
		return
	}
	var query sheets.Query
	if err := json.NewDecoder(r.Body).Decode(&query); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	result, err := h.svc.QueryRows(r.Context(), app.OrgID, pageID, query, sheets.QueryLimitREST)
	if err != nil {
		writeSheetsError(w, r, err)
		return
	}
	fields := make([]sheetFieldView, 0, len(result.Fields))
	for _, field := range result.Fields {
		fields = append(fields, sheetFieldViewFrom(field))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"rows":        sheetRowViewsFrom(result.Rows),
		"fields":      fields,
		"next_cursor": result.NextCursor,
		"relations":   result.Relations,
	})
}

// InsertRows handles POST /internal/apps/{appID}/v1/pages/{pageID}/rows
// (≤100 rows per call).
func (h *AppsInternalHandler) InsertRows(w http.ResponseWriter, r *http.Request) {
	app, ok := h.authApp(w, r)
	if !ok {
		return
	}
	pageID, ok := h.appPageID(w, r, app)
	if !ok {
		return
	}
	actor, ok := h.appActor(w, r, app)
	if !ok {
		return
	}
	var req insertRowsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	if len(req.Rows) == 0 {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "rows is required"})
		return
	}
	inserts := make([]sheets.RowInsert, 0, len(req.Rows))
	for _, row := range req.Rows {
		inserts = append(inserts, sheets.RowInsert{Data: row.Data, Position: row.Position})
	}
	ctx := sheets.WithMutationID(r.Context(), req.MutationID)
	rows, err := h.svc.InsertRows(ctx, app.OrgID, pageID, inserts, sheets.MaxRowsPerWriteMCP, actor)
	if err != nil {
		writeSheetsError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"rows": sheetRowViewsFrom(rows)})
}

// UpdateRows handles PATCH /internal/apps/{appID}/v1/pages/{pageID}/rows —
// partial merges per field key, like the sheets REST surface.
func (h *AppsInternalHandler) UpdateRows(w http.ResponseWriter, r *http.Request) {
	app, ok := h.authApp(w, r)
	if !ok {
		return
	}
	pageID, ok := h.appPageID(w, r, app)
	if !ok {
		return
	}
	actor, ok := h.appActor(w, r, app)
	if !ok {
		return
	}
	var req updateRowsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	if len(req.Rows) == 0 {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "rows is required"})
		return
	}
	updates := make([]sheets.RowUpdate, 0, len(req.Rows))
	for _, row := range req.Rows {
		if row.ID == uuid.Nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "each row needs an id"})
			return
		}
		updates = append(updates, sheets.RowUpdate{ID: row.ID, Data: row.Data, Position: row.Position})
	}
	ctx := sheets.WithMutationID(r.Context(), req.MutationID)
	rows, err := h.svc.UpdateRows(ctx, app.OrgID, pageID, updates, sheets.MaxRowsPerWriteMCP, actor)
	if err != nil {
		writeSheetsError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"rows": sheetRowViewsFrom(rows)})
}

// DeleteRows handles DELETE /internal/apps/{appID}/v1/pages/{pageID}/rows
// (archive, revertible like every sheet write).
func (h *AppsInternalHandler) DeleteRows(w http.ResponseWriter, r *http.Request) {
	app, ok := h.authApp(w, r)
	if !ok {
		return
	}
	pageID, ok := h.appPageID(w, r, app)
	if !ok {
		return
	}
	actor, ok := h.appActor(w, r, app)
	if !ok {
		return
	}
	var req deleteRowsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	if len(req.IDs) == 0 {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "ids is required"})
		return
	}
	ctx := sheets.WithMutationID(r.Context(), req.MutationID)
	archived, err := h.svc.DeleteRows(ctx, app.OrgID, pageID, req.IDs, sheets.MaxRowsPerWriteMCP, actor)
	if err != nil {
		writeSheetsError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"archived": archived})
}

// AttachmentDownloadURL handles
// POST /internal/apps/{appID}/v1/pages/{pageID}/attachments/download-url,
// re-checking org ownership of every key before signing — the same admission
// rule as the sheets REST surface.
func (h *AppsInternalHandler) AttachmentDownloadURL(w http.ResponseWriter, r *http.Request) {
	app, ok := h.authApp(w, r)
	if !ok {
		return
	}
	if h.presigner == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "attachment downloads are not configured"})
		return
	}
	if _, ok := h.appPageID(w, r, app); !ok {
		return
	}
	var req attachmentDownloadURLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	if len(req.Keys) == 0 || len(req.Keys) > maxDownloadURLKeys {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "keys must contain between 1 and 50 entries"})
		return
	}
	keys := make([]string, 0, len(req.Keys))
	for _, key := range req.Keys {
		keys = append(keys, strings.TrimSpace(key))
	}
	if err := h.svc.AuthorizeObjectKeys(r.Context(), app.OrgID, keys); err != nil {
		var attErr *sheets.AttachmentError
		if errors.As(err, &attErr) {
			writeJSON(w, http.StatusForbidden, errorResponse{Error: "object key is not owned by this org"})
			return
		}
		writeSheetsError(w, r, err)
		return
	}
	urls := make(map[string]string, len(keys))
	for _, key := range keys {
		url, err := h.presigner.PresignGet(r.Context(), key)
		if err != nil {
			writeSheetsError(w, r, err)
			return
		}
		urls[key] = url
	}
	writeJSON(w, http.StatusOK, map[string]any{"urls": urls})
}
