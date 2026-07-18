package handler

import (
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/apps"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

// AppsHandler serves the org-facing /v1/apps surface. Team authorization
// makes a denied app 404 so
// it is indistinguishable from a missing one.
type AppsHandler struct {
	db     *gorm.DB
	svc    *apps.Service
	rsaKey *rsa.PrivateKey
}

func NewAppsHandler(db *gorm.DB, svc *apps.Service, rsaKey *rsa.PrivateKey) *AppsHandler {
	return &AppsHandler{db: db, svc: svc, rsaKey: rsaKey}
}

type createAppRequest struct {
	TeamID      string `json:"team_id"`
	SheetID     string `json:"sheet_id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Icon        string `json:"icon,omitempty"`
}

type appView struct {
	ID              string     `json:"id"`
	TeamID          string     `json:"team_id"`
	SheetID         string     `json:"sheet_id"`
	Slug            string     `json:"slug"`
	Name            string     `json:"name"`
	Description     string     `json:"description"`
	Icon            string     `json:"icon"`
	Status          string     `json:"status"`
	ActiveVersionID *uuid.UUID `json:"active_version_id,omitempty"`
	TemplateVersion string     `json:"template_version"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type appVersionView struct {
	ID              string    `json:"id"`
	BundleSHA256    string    `json:"bundle_sha256"`
	SourceSHA256    string    `json:"source_sha256"`
	BundleBytes     int64     `json:"bundle_bytes"`
	SourceBytes     int64     `json:"source_bytes"`
	Notes           string    `json:"notes"`
	TemplateVersion string    `json:"template_version"`
	CreatedAt       time.Time `json:"created_at"`
}

// Response DTOs for OpenAPI generation (swaggo); the handlers write the same
// shapes as map literals.
type appListResponse struct {
	Apps []appView `json:"apps"`
}

type appDetailResponse struct {
	App      appView          `json:"app"`
	Versions []appVersionView `json:"versions"`
}

func appViewFrom(m *model.App) appView {
	return appView{
		ID:              m.ID.String(),
		TeamID:          m.TeamID.String(),
		SheetID:         m.SheetID.String(),
		Slug:            m.Slug,
		Name:            m.Name,
		Description:     m.Description,
		Icon:            m.Icon,
		Status:          m.Status,
		ActiveVersionID: m.ActiveVersionID,
		TemplateVersion: m.TemplateVersion,
		CreatedAt:       m.CreatedAt,
		UpdatedAt:       m.UpdatedAt,
	}
}

func appVersionViewFrom(m model.AppVersion) appVersionView {
	return appVersionView{
		ID:              m.ID.String(),
		BundleSHA256:    m.BundleSHA256,
		SourceSHA256:    m.SourceSHA256,
		BundleBytes:     m.BundleBytes,
		SourceBytes:     m.SourceBytes,
		Notes:           m.Notes,
		TemplateVersion: m.TemplateVersion,
		CreatedAt:       m.CreatedAt,
	}
}

// Create handles POST /v1/apps.
// @Summary Create an app
// @Description Registers a new app bound to exactly one sheet in one team. Returns 409 when an active app in the org already uses the slug derived from the name.
// @Tags apps
// @Accept json
// @Produce json
// @Param body body createAppRequest true "App to create"
// @Success 201 {object} appView
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Security BearerAuth
// @Router /v1/apps [post]
func (h *AppsHandler) Create(w http.ResponseWriter, r *http.Request) {
	org, ok := h.requireAppsOrg(w, r)
	if !ok {
		return
	}
	var req createAppRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	teamID, err := uuid.Parse(req.TeamID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "team_id must be a uuid"})
		return
	}
	sheetID, err := uuid.Parse(req.SheetID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "sheet_id must be a uuid"})
		return
	}
	if !h.canUseAppTeam(r.Context(), org.ID, teamID) {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
		return
	}

	params := apps.CreateAppParams{
		OrgID:       org.ID,
		TeamID:      teamID,
		SheetID:     sheetID,
		Name:        req.Name,
		Description: req.Description,
		Icon:        req.Icon,
	}
	if user, ok := middleware.UserFromContext(r.Context()); ok && user != nil {
		params.CreatedByUserID = &user.ID
	}
	app, err := h.svc.CreateApp(r.Context(), params)
	if err != nil {
		writeAppsError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, appViewFrom(app))
}

// List handles GET /v1/apps.
// @Summary List a team's apps
// @Tags apps
// @Produce json
// @Param team_id query string true "Team ID"
// @Success 200 {object} appListResponse
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/apps [get]
func (h *AppsHandler) List(w http.ResponseWriter, r *http.Request) {
	org, ok := h.requireAppsOrg(w, r)
	if !ok {
		return
	}
	teamID, err := uuid.Parse(r.URL.Query().Get("team_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "team_id query parameter must be a uuid"})
		return
	}
	if !h.canUseAppTeam(r.Context(), org.ID, teamID) {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
		return
	}
	list, err := h.svc.ListApps(r.Context(), org.ID, teamID)
	if err != nil {
		writeAppsError(w, r, err)
		return
	}
	views := make([]appView, 0, len(list))
	for i := range list {
		views = append(views, appViewFrom(&list[i]))
	}
	writeJSON(w, http.StatusOK, map[string]any{"apps": views})
}
