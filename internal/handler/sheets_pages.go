package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sheets"
)

type createPageRequest struct {
	Name       string                  `json:"name"`
	Fields     []sheetFieldSpecRequest `json:"fields,omitempty"`
	MutationID string                  `json:"mutation_id,omitempty"`
}

func (h *SheetsHandler) CreatePage(w http.ResponseWriter, r *http.Request) {
	org, ok := h.requireSheetsOrg(w, r)
	if !ok {
		return
	}
	sheetID, ok := sheetsPathUUID(w, r, "sheetID")
	if !ok {
		return
	}
	var req createPageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "name is required"})
		return
	}
	ctx := sheets.WithMutationID(r.Context(), req.MutationID)
	structure, err := h.svc.CreatePage(ctx, org.ID, sheetID, sheets.PageSpec{
		Name: req.Name, Fields: fieldSpecsFrom(req.Fields),
	})
	if err != nil {
		writeSheetsError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, sheetPageViewFrom(structure.Page, structure.Fields, 0))
}

type updatePageRequest struct {
	Name           *string  `json:"name,omitempty"`
	Position       *float64 `json:"position,omitempty"`
	DisplayFieldID *string  `json:"display_field_id,omitempty"`
	MutationID     string   `json:"mutation_id,omitempty"`
}

func (h *SheetsHandler) UpdatePage(w http.ResponseWriter, r *http.Request) {
	org, ok := h.requireSheetsOrg(w, r)
	if !ok {
		return
	}
	pageID, ok := h.sheetsNestedPageID(w, r, org)
	if !ok {
		return
	}
	var req updatePageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	ctx := sheets.WithMutationID(r.Context(), req.MutationID)
	page, err := h.svc.UpdatePage(ctx, org.ID, pageID, sheets.UpdatePageRequest{
		Name: req.Name, Position: req.Position, DisplayFieldID: req.DisplayFieldID,
	})
	if err != nil {
		writeSheetsError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, sheetPageViewFrom(*page, nil, 0))
}

func (h *SheetsHandler) ArchivePage(w http.ResponseWriter, r *http.Request) {
	org, ok := h.requireSheetsOrg(w, r)
	if !ok {
		return
	}
	pageID, ok := h.sheetsNestedPageID(w, r, org)
	if !ok {
		return
	}
	if err := h.svc.ArchivePage(r.Context(), org.ID, pageID); err != nil {
		writeSheetsError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "archived"})
}

type createFieldRequest struct {
	Name       string         `json:"name"`
	Type       string         `json:"type"`
	Options    map[string]any `json:"options,omitempty"`
	MutationID string         `json:"mutation_id,omitempty"`
}

func (h *SheetsHandler) CreateField(w http.ResponseWriter, r *http.Request) {
	org, ok := h.requireSheetsOrg(w, r)
	if !ok {
		return
	}
	pageID, ok := h.sheetsNestedPageID(w, r, org)
	if !ok {
		return
	}
	var req createFieldRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	ctx := sheets.WithMutationID(r.Context(), req.MutationID)
	field, err := h.svc.CreateField(ctx, org.ID, pageID, sheets.FieldSpec{
		Name: req.Name, Type: req.Type, Options: model.JSON(req.Options),
	}, sheetsActor(r))
	if err != nil {
		writeSheetsError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, sheetFieldViewFrom(*field))
}

type updateFieldRequest struct {
	Name       *string         `json:"name,omitempty"`
	Type       *string         `json:"type,omitempty"`
	Options    *map[string]any `json:"options,omitempty"`
	Position   *float64        `json:"position,omitempty"`
	MutationID string          `json:"mutation_id,omitempty"`
}

func (h *SheetsHandler) UpdateField(w http.ResponseWriter, r *http.Request) {
	org, ok := h.requireSheetsOrg(w, r)
	if !ok {
		return
	}
	if _, ok := h.sheetsNestedPageID(w, r, org); !ok {
		return
	}
	fieldID := chi.URLParam(r, "fieldID")
	if !sheets.ValidFieldID(fieldID) {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "fieldID must be a field id"})
		return
	}
	var req updateFieldRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	update := sheets.UpdateFieldRequest{
		Name: req.Name, Type: req.Type, Position: req.Position,
	}
	if req.Options != nil {
		options := model.JSON(*req.Options)
		update.Options = &options
	}
	ctx := sheets.WithMutationID(r.Context(), req.MutationID)
	field, err := h.svc.UpdateField(ctx, org.ID, fieldID, update, sheetsActor(r))
	if err != nil {
		writeSheetsError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, sheetFieldViewFrom(*field))
}

func (h *SheetsHandler) ArchiveField(w http.ResponseWriter, r *http.Request) {
	org, ok := h.requireSheetsOrg(w, r)
	if !ok {
		return
	}
	if _, ok := h.sheetsNestedPageID(w, r, org); !ok {
		return
	}
	fieldID := chi.URLParam(r, "fieldID")
	if !sheets.ValidFieldID(fieldID) {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "fieldID must be a field id"})
		return
	}
	if err := h.svc.ArchiveField(r.Context(), org.ID, fieldID, sheetsActor(r)); err != nil {
		writeSheetsError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "archived"})
}
