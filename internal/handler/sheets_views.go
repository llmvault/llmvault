package handler

import (
	"encoding/json"
	"net/http"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sheets"
)

// ListViews handles GET /v1/sheets/{sheetID}/pages/{pageID}/views.
// @Summary List views
// @Tags sheets
// @Produce json
// @Param sheetID path string true "Sheet ID"
// @Param pageID path string true "Page ID"
// @Success 200 {object} sheetViewsResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/sheets/{sheetID}/pages/{pageID}/views [get]
func (h *SheetsHandler) ListViews(w http.ResponseWriter, r *http.Request) {
	org, ok := h.requireSheetsOrg(w, r)
	if !ok {
		return
	}
	pageID, ok := h.sheetsNestedPageID(w, r, org)
	if !ok {
		return
	}
	views, err := h.svc.ListViews(r.Context(), org.ID, pageID)
	if err != nil {
		writeSheetsError(w, r, err)
		return
	}
	out := make([]sheetViewSummary, 0, len(views))
	for _, view := range views {
		out = append(out, sheetViewSummaryFrom(view))
	}
	writeJSON(w, http.StatusOK, map[string]any{"views": out})
}

type createViewRequest struct {
	Name   string         `json:"name"`
	Type   string         `json:"type,omitempty"`
	Config map[string]any `json:"config,omitempty"`
}

// CreateView handles POST /v1/sheets/{sheetID}/pages/{pageID}/views.
// @Summary Create a view
// @Tags sheets
// @Accept json
// @Produce json
// @Param sheetID path string true "Sheet ID"
// @Param pageID path string true "Page ID"
// @Param body body createViewRequest true "View to create"
// @Success 201 {object} sheetViewSummary
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/sheets/{sheetID}/pages/{pageID}/views [post]
func (h *SheetsHandler) CreateView(w http.ResponseWriter, r *http.Request) {
	org, ok := h.requireSheetsOrg(w, r)
	if !ok {
		return
	}
	pageID, ok := h.sheetsNestedPageID(w, r, org)
	if !ok {
		return
	}
	var req createViewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	view, err := h.svc.CreateView(r.Context(), org.ID, pageID, sheets.ViewSpec{
		Name: req.Name, Type: req.Type, Config: model.JSON(req.Config),
	})
	if err != nil {
		writeSheetsError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, sheetViewSummaryFrom(*view))
}

type updateViewRequest struct {
	Name     *string         `json:"name,omitempty"`
	Config   *map[string]any `json:"config,omitempty"`
	Position *float64        `json:"position,omitempty"`
}

// UpdateView handles PATCH /v1/sheets/{sheetID}/pages/{pageID}/views/{viewID}.
// @Summary Update a view
// @Tags sheets
// @Accept json
// @Produce json
// @Param sheetID path string true "Sheet ID"
// @Param pageID path string true "Page ID"
// @Param viewID path string true "View ID"
// @Param body body updateViewRequest true "Fields to update"
// @Success 200 {object} sheetViewSummary
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/sheets/{sheetID}/pages/{pageID}/views/{viewID} [patch]
func (h *SheetsHandler) UpdateView(w http.ResponseWriter, r *http.Request) {
	org, ok := h.requireSheetsOrg(w, r)
	if !ok {
		return
	}
	pageID, ok := h.sheetsNestedPageID(w, r, org)
	if !ok {
		return
	}
	viewID, ok := sheetsPathUUID(w, r, "viewID")
	if !ok {
		return
	}
	if err := h.svc.ViewInPage(r.Context(), org.ID, pageID, viewID); err != nil {
		writeSheetsError(w, r, err)
		return
	}
	var req updateViewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	update := sheets.UpdateViewRequest{Name: req.Name, Position: req.Position}
	if req.Config != nil {
		config := model.JSON(*req.Config)
		update.Config = &config
	}
	view, err := h.svc.UpdateView(r.Context(), org.ID, viewID, update)
	if err != nil {
		writeSheetsError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, sheetViewSummaryFrom(*view))
}

// ArchiveView handles DELETE /v1/sheets/{sheetID}/pages/{pageID}/views/{viewID}.
// @Summary Archive a view
// @Tags sheets
// @Produce json
// @Param sheetID path string true "Sheet ID"
// @Param pageID path string true "Page ID"
// @Param viewID path string true "View ID"
// @Success 200 {object} sheetStatusResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/sheets/{sheetID}/pages/{pageID}/views/{viewID} [delete]
func (h *SheetsHandler) ArchiveView(w http.ResponseWriter, r *http.Request) {
	org, ok := h.requireSheetsOrg(w, r)
	if !ok {
		return
	}
	pageID, ok := h.sheetsNestedPageID(w, r, org)
	if !ok {
		return
	}
	viewID, ok := sheetsPathUUID(w, r, "viewID")
	if !ok {
		return
	}
	if err := h.svc.ViewInPage(r.Context(), org.ID, pageID, viewID); err != nil {
		writeSheetsError(w, r, err)
		return
	}
	if err := h.svc.ArchiveView(r.Context(), org.ID, viewID); err != nil {
		writeSheetsError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "archived"})
}
