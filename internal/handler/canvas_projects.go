package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"

	canvaspkg "github.com/usehivy/hivy/internal/canvas"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/middleware"
)

type canvasProjectCatalogResponse struct {
	Projects []canvasProjectCatalogProjectResponse `json:"projects"`
}

type canvasProjectCatalogProjectResponse struct {
	ProjectID uuid.UUID                          `json:"project_id"`
	Name      string                             `json:"name"`
	Files     []canvasProjectCatalogFileResponse `json:"files"`
}

type canvasProjectCatalogFileResponse struct {
	FileID       uuid.UUID  `json:"file_id"`
	ProjectID    uuid.UUID  `json:"project_id"`
	PageID       *uuid.UUID `json:"page_id,omitempty"`
	Name         string     `json:"name"`
	WorkspaceURL string     `json:"workspace_url"`
}

// ListProjects handles GET /v1/canvas/projects.
// @Summary List Canvas projects
// @Description Returns Canvas projects and their files for the current organization.
// @Tags canvas
// @Produce json
// @Success 200 {object} canvasProjectCatalogResponse
// @Failure 401 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Failure 503 {object} errorResponse
// @Security BearerAuth
// @Router /v1/canvas/projects [get]
func (h *CanvasHandler) ListProjects(w http.ResponseWriter, r *http.Request) {
	if h == nil || (h.svc == nil && h.artifactSvc == nil) {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "canvas is not configured"})
		return
	}
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok || org == nil {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing org context"})
		return
	}
	if h.artifactSvc != nil {
		var sessionID *uuid.UUID
		rawSessionID := strings.TrimSpace(r.URL.Query().Get("session_id"))
		if rawSessionID != "" {
			parsed, err := uuid.Parse(rawSessionID)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, errorResponse{Error: "session_id must be a uuid"})
				return
			}
			sessionID = &parsed
		}
		result, err := h.artifactSvc.ListProjectsForOrg(r.Context(), org.ID, sessionID)
		if err != nil {
			logging.Capture(r.Context(), err)
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to list canvas projects"})
			return
		}
		writeJSON(w, http.StatusOK, result)
		return
	}
	result, err := h.svc.ListProjectCatalogForOrg(r.Context(), org.ID)
	if err != nil {
		if errors.Is(err, canvaspkg.ErrNotConfigured) {
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "canvas is not configured"})
			return
		}
		logging.Capture(r.Context(), err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to list canvas projects"})
		return
	}
	writeJSON(w, http.StatusOK, result)
}
