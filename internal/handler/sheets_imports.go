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

func (h *SheetsHandler) GetImportJob(w http.ResponseWriter, r *http.Request) {
	org, ok := h.requireSheetsOrg(w, r)
	if !ok {
		return
	}
	jobID, ok := sheetsPathUUID(w, r, "jobID")
	if !ok {
		return
	}
	channelID, err := h.svc.ChannelForImportJob(r.Context(), org.ID, jobID)
	if err != nil {
		writeSheetsError(w, r, err)
		return
	}
	if !h.canUseSheetChannel(r.Context(), org.ID, channelID) {
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

func (h *SheetsHandler) RevertOperation(w http.ResponseWriter, r *http.Request) {
	org, ok := h.requireSheetsOrg(w, r)
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
