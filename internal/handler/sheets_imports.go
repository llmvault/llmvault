package handler

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sheets"
)

type createImportRequest struct {
	ObjectKey  string         `json:"object_key"`
	Options    map[string]any `json:"options,omitempty"`
	MutationID string         `json:"mutation_id,omitempty"`
}

// CreateImport handles POST /v1/sheets/{sheetID}/pages/{pageID}/imports.
// @Summary Start a CSV import
// @Description Starts an asynchronous CSV import from a drive object key into a page.
// @Tags sheets
// @Accept json
// @Produce json
// @Param sheetID path string true "Sheet ID"
// @Param pageID path string true "Page ID"
// @Param body body createImportRequest true "Import to start"
// @Success 201 {object} sheetImportJobView
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/sheets/{sheetID}/pages/{pageID}/imports [post]
func (h *SheetsHandler) CreateImport(w http.ResponseWriter, r *http.Request) {
	org, ok := h.requireSheetsOrg(w, r)
	if !ok {
		return
	}
	pageID, ok := h.sheetsNestedPageID(w, r, org)
	if !ok {
		return
	}
	var req createImportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	if req.ObjectKey == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "object_key is required"})
		return
	}
	ctx := sheets.WithMutationID(r.Context(), req.MutationID)
	job, err := h.svc.CreateImportJob(ctx, org.ID, pageID, sheets.CreateImportJobRequest{
		ObjectKey: req.ObjectKey, Options: model.JSON(req.Options),
	}, sheetsActor(r))
	if err != nil {
		writeSheetsError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, sheetImportJobViewFrom(*job))
}

// GetImportJob handles GET /v1/sheets/imports/{jobID}.
// @Summary Get a CSV import job
// @Description Returns import job status. The caller must be able to use the job's sheet's team.
// @Tags sheets
// @Produce json
// @Param jobID path string true "Import job ID"
// @Success 200 {object} sheetImportJobView
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/sheets/imports/{jobID} [get]
func (h *SheetsHandler) GetImportJob(w http.ResponseWriter, r *http.Request) {
	org, ok := h.requireSheetsOrg(w, r)
	if !ok {
		return
	}
	jobID, ok := sheetsPathUUID(w, r, "jobID")
	if !ok {
		return
	}
	teamID, err := h.svc.TeamForImportJob(r.Context(), org.ID, jobID)
	if err != nil {
		writeSheetsError(w, r, err)
		return
	}
	if !h.canUseSheetTeam(r.Context(), org.ID, teamID) {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
		return
	}
	job, err := h.svc.GetImportJob(r.Context(), org.ID, jobID)
	if err != nil {
		writeSheetsError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, sheetImportJobViewFrom(*job))
}

// ExportCSV streams a page's rows as CSV, cursor-walking the same query path
// the grid uses so exports never load a whole page into memory.
// @Summary Export a page as CSV
// @Tags sheets
// @Produce text/csv
// @Param sheetID path string true "Sheet ID"
// @Param pageID path string true "Page ID"
// @Success 200 {string} string "CSV stream"
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/sheets/{sheetID}/pages/{pageID}/export.csv [get]
func (h *SheetsHandler) ExportCSV(w http.ResponseWriter, r *http.Request) {
	org, ok := h.requireSheetsOrg(w, r)
	if !ok {
		return
	}
	pageID, ok := h.sheetsNestedPageID(w, r, org)
	if !ok {
		return
	}
	first, err := h.svc.QueryRows(r.Context(), org.ID, pageID, sheets.Query{Limit: sheets.QueryLimitREST}, sheets.QueryLimitREST)
	if err != nil {
		writeSheetsError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="sheet-export.csv"`)
	w.WriteHeader(http.StatusOK)
	writer := csv.NewWriter(w)
	header := make([]string, 0, len(first.Fields)+1)
	header = append(header, "id")
	for _, field := range first.Fields {
		header = append(header, field.Name)
	}
	_ = writer.Write(header)

	result := first
	for {
		for _, row := range result.Rows {
			record := make([]string, 0, len(first.Fields)+1)
			record = append(record, row.ID.String())
			for _, field := range first.Fields {
				record = append(record, csvCellString(row.Data[field.ID]))
			}
			if err := writer.Write(record); err != nil {
				return
			}
		}
		writer.Flush()
		if writer.Error() != nil || result.NextCursor == "" {
			return
		}
		result, err = h.svc.QueryRows(r.Context(), org.ID, pageID, sheets.Query{
			Limit: sheets.QueryLimitREST, Cursor: result.NextCursor,
		}, sheets.QueryLimitREST)
		if err != nil {
			return
		}
	}
}

func csvCellString(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case bool:
		return strconv.FormatBool(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case []any, map[string]any:
		encoded, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		return string(encoded)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// ListOperations handles GET /v1/sheets/{sheetID}/pages/{pageID}/operations.
// @Summary List page operations
// @Description Returns a page's recent revertible operations, newest first.
// @Tags sheets
// @Produce json
// @Param sheetID path string true "Sheet ID"
// @Param pageID path string true "Page ID"
// @Param limit query int false "Max operations to return"
// @Success 200 {object} sheetOperationsResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/sheets/{sheetID}/pages/{pageID}/operations [get]
func (h *SheetsHandler) ListOperations(w http.ResponseWriter, r *http.Request) {
	org, ok := h.requireSheetsOrg(w, r)
	if !ok {
		return
	}
	pageID, ok := h.sheetsNestedPageID(w, r, org)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	ops, err := h.svc.ListOperations(r.Context(), org.ID, pageID, limit)
	if err != nil {
		writeSheetsError(w, r, err)
		return
	}
	out := make([]sheetOperationView, 0, len(ops))
	for _, op := range ops {
		out = append(out, sheetOperationViewFrom(op))
	}
	writeJSON(w, http.StatusOK, map[string]any{"operations": out})
}

type revertOperationRequest struct {
	MutationID string `json:"mutation_id,omitempty"`
}

// RevertOperation handles POST /v1/sheets/{sheetID}/pages/{pageID}/operations/{operationID}/revert.
// @Summary Revert an operation
// @Description Applies one operation's exact inverse. Single-level undo — a revert is not itself re-undoable.
// @Tags sheets
// @Accept json
// @Produce json
// @Param sheetID path string true "Sheet ID"
// @Param pageID path string true "Page ID"
// @Param operationID path string true "Operation ID"
// @Success 200 {object} sheetStatusResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Security BearerAuth
// @Router /v1/sheets/{sheetID}/pages/{pageID}/operations/{operationID}/revert [post]
func (h *SheetsHandler) RevertOperation(w http.ResponseWriter, r *http.Request) {
	org, ok := h.requireSheetsOrg(w, r)
	if !ok {
		return
	}
	sheetID, ok := sheetsPathUUID(w, r, "sheetID")
	if !ok {
		return
	}
	if _, ok := h.sheetsNestedPageID(w, r, org); !ok {
		return
	}
	operationID, ok := sheetsPathUUID(w, r, "operationID")
	if !ok {
		return
	}
	// RequireTeamAccess authorized the path sheet's team, but RevertOperation
	// mutates by operationID scoped only by org — so a caller could pass a sheet
	// they can access plus an operationID from a team they cannot. Bind the
	// operation to the path sheet's team first (the guard the MCP path uses).
	teamID, err := h.svc.TeamForSheet(r.Context(), org.ID, sheetID)
	if err != nil {
		writeSheetsError(w, r, err)
		return
	}
	if err := h.svc.OperationInTeam(r.Context(), org.ID, teamID, operationID); err != nil {
		writeSheetsError(w, r, err)
		return
	}
	var req revertOperationRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	ctx := sheets.WithMutationID(r.Context(), req.MutationID)
	if err := h.svc.RevertOperation(ctx, org.ID, operationID, sheetsActor(r)); err != nil {
		writeSheetsError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "reverted"})
}
