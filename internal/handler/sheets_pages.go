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

// CreatePage handles POST /v1/sheets/{sheetID}/pages.
// @Summary Create a page
// @Tags sheets
// @Accept json
// @Produce json
// @Param sheetID path string true "Sheet ID"
// @Param body body createPageRequest true "Page to create"
// @Success 201 {object} sheetPageView
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/sheets/{sheetID}/pages [post]
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

// UpdatePage handles PATCH /v1/sheets/{sheetID}/pages/{pageID}.
// @Summary Update a page
// @Tags sheets
// @Accept json
// @Produce json
// @Param sheetID path string true "Sheet ID"
// @Param pageID path string true "Page ID"
// @Param body body updatePageRequest true "Fields to update"
// @Success 200 {object} sheetPageView
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/sheets/{sheetID}/pages/{pageID} [patch]
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

// ArchivePage handles DELETE /v1/sheets/{sheetID}/pages/{pageID}.
// @Summary Archive a page
// @Tags sheets
// @Produce json
// @Param sheetID path string true "Sheet ID"
// @Param pageID path string true "Page ID"
// @Success 200 {object} sheetStatusResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/sheets/{sheetID}/pages/{pageID} [delete]
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

// CreateField handles POST /v1/sheets/{sheetID}/pages/{pageID}/fields.
// @Summary Create a field
// @Tags sheets
// @Accept json
// @Produce json
// @Param sheetID path string true "Sheet ID"
// @Param pageID path string true "Page ID"
// @Param body body createFieldRequest true "Field to create"
// @Success 201 {object} sheetFieldView
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/sheets/{sheetID}/pages/{pageID}/fields [post]
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

// UpdateField handles PATCH /v1/sheets/{sheetID}/pages/{pageID}/fields/{fieldID}.
// @Summary Update a field
// @Tags sheets
// @Accept json
// @Produce json
// @Param sheetID path string true "Sheet ID"
// @Param pageID path string true "Page ID"
// @Param fieldID path string true "Field ID"
// @Param body body updateFieldRequest true "Fields to update"
// @Success 200 {object} sheetFieldView
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/sheets/{sheetID}/pages/{pageID}/fields/{fieldID} [patch]
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

// ArchiveField handles DELETE /v1/sheets/{sheetID}/pages/{pageID}/fields/{fieldID}.
// @Summary Archive a field
// @Tags sheets
// @Produce json
// @Param sheetID path string true "Sheet ID"
// @Param pageID path string true "Page ID"
// @Param fieldID path string true "Field ID"
// @Success 200 {object} sheetStatusResponse
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/sheets/{sheetID}/pages/{pageID}/fields/{fieldID} [delete]
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
