package handler

import (
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/logging"
)

// ListProjects handles GET /v1/canvas/projects.
// @Summary List Canvas projects
// @Description Returns Canvas artifact projects for the current organization.
// @Tags canvas
// @Produce json
// @Success 200 {object} canvasartifact.ProjectListResponse
// @Failure 401 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Failure 503 {object} errorResponse
// @Security BearerAuth
// @Router /v1/canvas/projects [get]
func (h *CanvasHandler) ListProjects(w http.ResponseWriter, r *http.Request) {
	org, ok := h.requireCanvasArtifactOrg(w, r)
	if !ok || org == nil {
		return
	}
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
}
