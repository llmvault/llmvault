package handler

import (
	"errors"
	"net/http"

	canvaspkg "github.com/usehivy/hivy/internal/canvas"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/middleware"
)

// ListProjects handles GET /v1/canvas/projects.
func (h *CanvasHandler) ListProjects(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.svc == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "canvas is not configured"})
		return
	}
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok || org == nil {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing org context"})
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
