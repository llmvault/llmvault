package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/sheets"
)

// maxDownloadURLKeys caps one download-url request.
const maxDownloadURLKeys = 50

// QueryRows handles POST /v1/sheets/{sheetID}/pages/{pageID}/rows/query.
// @Summary Query rows
// @Description Query a page's rows with a filter AST, sorts, search, and cursor paging.
// @Tags sheets
// @Accept json
// @Produce json
// @Param sheetID path string true "Sheet ID"
// @Param pageID path string true "Page ID"
// @Param body body sheets.Query true "Query"
// @Success 200 {object} sheetRowsQueryResponse
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/sheets/{sheetID}/pages/{pageID}/rows/query [post]
func (h *SheetsHandler) QueryRows(w http.ResponseWriter, r *http.Request) {
	org, ok := h.requireSheetsOrg(w, r)
	if !ok {
		return
	}
	pageID, ok := h.sheetsNestedPageID(w, r, org)
	if !ok {
		return
	}
	var query sheets.Query
	if err := json.NewDecoder(r.Body).Decode(&query); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	result, err := h.svc.QueryRows(r.Context(), org.ID, pageID, query, sheets.QueryLimitREST)
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

type insertRowRequest struct {
	Data     map[string]any `json:"data"`
	Position *float64       `json:"position,omitempty"`
}

type insertRowsRequest struct {
	Rows       []insertRowRequest `json:"rows"`
	MutationID string             `json:"mutation_id,omitempty"`
}

// InsertRows handles POST /v1/sheets/{sheetID}/pages/{pageID}/rows.
// @Summary Insert rows
// @Tags sheets
// @Accept json
// @Produce json
// @Param sheetID path string true "Sheet ID"
// @Param pageID path string true "Page ID"
// @Param body body insertRowsRequest true "Rows to insert"
// @Success 201 {object} sheetRowsResponse
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/sheets/{sheetID}/pages/{pageID}/rows [post]
func (h *SheetsHandler) InsertRows(w http.ResponseWriter, r *http.Request) {
	org, ok := h.requireSheetsOrg(w, r)
	if !ok {
		return
	}
	pageID, ok := h.sheetsNestedPageID(w, r, org)
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
	rows, err := h.svc.InsertRows(ctx, org.ID, pageID, inserts, sheets.MaxRowsPerWriteREST, sheetsActor(r))
	if err != nil {
		writeSheetsError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"rows": sheetRowViewsFrom(rows)})
}

type updateRowRequest struct {
	ID       uuid.UUID      `json:"id"`
	Data     map[string]any `json:"data"`
	Position *float64       `json:"position,omitempty"`
}

type updateRowsRequest struct {
	Rows       []updateRowRequest `json:"rows"`
	MutationID string             `json:"mutation_id,omitempty"`
}

// UpdateRows applies partial merges per field key; a single cell edit is a
// batch of one row with one key. Row drag-reorder is the same endpoint with
// position set.
// @Summary Update rows
// @Description Partial-merges each row's data by field key. A single cell edit is a batch of one.
// @Tags sheets
// @Accept json
// @Produce json
// @Param sheetID path string true "Sheet ID"
// @Param pageID path string true "Page ID"
// @Param body body updateRowsRequest true "Rows to update"
// @Success 200 {object} sheetRowsResponse
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/sheets/{sheetID}/pages/{pageID}/rows [patch]
func (h *SheetsHandler) UpdateRows(w http.ResponseWriter, r *http.Request) {
	org, ok := h.requireSheetsOrg(w, r)
	if !ok {
		return
	}
	pageID, ok := h.sheetsNestedPageID(w, r, org)
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
	rows, err := h.svc.UpdateRows(ctx, org.ID, pageID, updates, sheets.MaxRowsPerWriteREST, sheetsActor(r))
	if err != nil {
		writeSheetsError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"rows": sheetRowViewsFrom(rows)})
}

type deleteRowsRequest struct {
	IDs        []uuid.UUID `json:"ids"`
	MutationID string      `json:"mutation_id,omitempty"`
}

// DeleteRows handles DELETE /v1/sheets/{sheetID}/pages/{pageID}/rows.
// @Summary Delete rows
// @Tags sheets
// @Accept json
// @Produce json
// @Param sheetID path string true "Sheet ID"
// @Param pageID path string true "Page ID"
// @Param body body deleteRowsRequest true "Row IDs to archive"
// @Success 200 {object} sheetArchivedRowsResponse
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/sheets/{sheetID}/pages/{pageID}/rows [delete]
func (h *SheetsHandler) DeleteRows(w http.ResponseWriter, r *http.Request) {
	org, ok := h.requireSheetsOrg(w, r)
	if !ok {
		return
	}
	pageID, ok := h.sheetsNestedPageID(w, r, org)
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
	archived, err := h.svc.DeleteRows(ctx, org.ID, pageID, req.IDs, sheets.MaxRowsPerWriteREST, sheetsActor(r))
	if err != nil {
		writeSheetsError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"archived": archived})
}

type attachmentDownloadURLRequest struct {
	Keys []string `json:"keys"`
}

// AttachmentDownloadURL mints presigned GET URLs for attachment object keys,
// re-checking org ownership of every key before signing.
// @Summary Presign attachment downloads
// @Tags sheets
// @Accept json
// @Produce json
// @Param sheetID path string true "Sheet ID"
// @Param pageID path string true "Page ID"
// @Param body body attachmentDownloadURLRequest true "Object keys to presign (1-50)"
// @Success 200 {object} sheetAttachmentURLsResponse
// @Failure 400 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/sheets/{sheetID}/pages/{pageID}/attachments/download-url [post]
func (h *SheetsHandler) AttachmentDownloadURL(w http.ResponseWriter, r *http.Request) {
	org, ok := h.requireSheetsOrg(w, r)
	if !ok {
		return
	}
	if h.presigner == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "attachment downloads are not configured"})
		return
	}
	if _, ok := h.sheetsNestedPageID(w, r, org); !ok {
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
	// Same admission rule that let the keys into cells — org-owned keys plus
	// org-agent drive keys (sheets.Service.AuthorizeObjectKeys, one batched
	// agents lookup). The two contracts must stay identical.
	if err := h.svc.AuthorizeObjectKeys(r.Context(), org.ID, keys); err != nil {
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
