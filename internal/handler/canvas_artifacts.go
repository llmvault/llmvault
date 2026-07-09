package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

type canvasArtifactPreviewRequest struct {
	SessionID string `json:"session_id,omitempty"`
}

type canvasArtifactPreviewResponse struct {
	URL            string `json:"url"`
	SandboxBaseURL string `json:"sandbox_base_url"`
	Token          string `json:"token"`
	ExpiresAt      string `json:"expires_at"`
}

// ListArtifacts handles GET /v1/canvas/artifacts.
// @Summary List Canvas artifacts
// @Description Returns Canvas artifacts for the current organization, optionally filtered by project or session.
// @Tags canvas
// @Produce json
// @Param project_id query string false "Filter by project ID (uuid)"
// @Param project_slug query string false "Filter by project slug"
// @Param session_id query string false "Filter by source session ID (uuid)"
// @Success 200 {object} canvasartifact.ArtifactListResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Failure 503 {object} errorResponse
// @Security BearerAuth
// @Router /v1/canvas/artifacts [get]
func (h *CanvasHandler) ListArtifacts(w http.ResponseWriter, r *http.Request) {
	org, ok := h.requireCanvasArtifactOrg(w, r)
	if !ok {
		return
	}
	filter, ok := parseCanvasArtifactFilter(w, r)
	if !ok {
		return
	}
	visibleSessions, err := h.visibleSessionScope(r.Context(), org.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to resolve access"})
		return
	}
	filter.VisibleSessionIDs = visibleSessions
	result, err := h.artifactSvc.ListArtifactsForOrg(r.Context(), org.ID, filter)
	if err != nil {
		logging.Capture(r.Context(), err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to list canvas artifacts"})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// GetArtifact handles GET /v1/canvas/artifacts/{artifactID}.
// @Summary Get a Canvas artifact
// @Description Returns a single Canvas artifact for the current organization.
// @Tags canvas
// @Produce json
// @Param artifactID path string true "Artifact ID (uuid)"
// @Success 200 {object} canvasartifact.ArtifactResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Failure 503 {object} errorResponse
// @Security BearerAuth
// @Router /v1/canvas/artifacts/{artifactID} [get]
func (h *CanvasHandler) GetArtifact(w http.ResponseWriter, r *http.Request) {
	org, ok := h.requireCanvasArtifactOrg(w, r)
	if !ok {
		return
	}
	artifactID, err := uuid.Parse(chi.URLParam(r, "artifactID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "artifact_id must be a uuid"})
		return
	}
	visibleSessions, err := h.visibleSessionScope(r.Context(), org.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to resolve access"})
		return
	}
	result, err := h.artifactSvc.GetArtifactForOrg(r.Context(), org.ID, artifactID, visibleSessions)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "canvas artifact not found"})
			return
		}
		logging.Capture(r.Context(), err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load canvas artifact"})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// PreviewArtifactURL handles POST /v1/canvas/artifacts/{artifactID}/preview-url.
// @Summary Mint a Canvas artifact preview URL
// @Description Mints a short-lived sandbox preview URL for a Canvas artifact tied to a session.
// @Tags canvas
// @Accept json
// @Produce json
// @Param artifactID path string true "Artifact ID (uuid)"
// @Param body body canvasArtifactPreviewRequest true "Preview request payload"
// @Success 200 {object} canvasArtifactPreviewResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Failure 503 {object} errorResponse
// @Security BearerAuth
// @Router /v1/canvas/artifacts/{artifactID}/preview-url [post]
func (h *CanvasHandler) PreviewArtifactURL(w http.ResponseWriter, r *http.Request) {
	org, ok := h.requireCanvasArtifactOrg(w, r)
	if !ok {
		return
	}
	if h.encKey == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "runtime sandbox access is not configured"})
		return
	}
	artifactID, err := uuid.Parse(chi.URLParam(r, "artifactID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "artifact_id must be a uuid"})
		return
	}
	var req canvasArtifactPreviewRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	rawSessionID := strings.TrimSpace(req.SessionID)
	if rawSessionID == "" {
		rawSessionID = strings.TrimSpace(r.URL.Query().Get("session_id"))
	}
	sessionID, err := uuid.Parse(rawSessionID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "session_id must be a uuid"})
		return
	}
	visibleSessions, err := h.visibleSessionScope(r.Context(), org.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to resolve access"})
		return
	}
	artifactQ := h.db.WithContext(r.Context()).
		Preload("CanvasProject").
		Where("org_id = ? AND id = ? AND archived_at IS NULL", org.ID, artifactID)
	if visibleSessions != nil {
		// A member may only mint a preview for an artifact tied to a session
		// whose channel they can view; others are 404 as if nonexistent.
		artifactQ = artifactQ.Where("source_session_id IN (?)", visibleSessions)
	}
	var artifact model.CanvasArtifact
	if err := artifactQ.First(&artifact).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "canvas artifact not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load canvas artifact"})
		return
	}
	if artifact.SourceSessionID == nil || *artifact.SourceSessionID != sessionID {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "canvas artifact not found"})
		return
	}
	entryPath, err := h.previewEntryPath(r, artifact)
	if err != nil {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "canvas artifact preview file not found"})
		return
	}
	session, sb, err := h.canvasPreviewSandbox(r, org.ID, sessionID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "session sandbox not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load session sandbox"})
		return
	}
	runtimeSecret, err := h.encKey.DecryptString(sb.EncryptedRuntimeSecret)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "runtime sandbox access is not available"})
		return
	}
	expiresAt := time.Now().UTC().Add(time.Hour)
	token, err := canvasPreviewToken(runtimeSecret, session, sb, expiresAt)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to mint sandbox access"})
		return
	}
	baseURL := strings.TrimRight(sb.RuntimeURL, "/")
	previewPath := "projects/" + artifact.CanvasProject.Slug + "/artifacts/" + artifact.Slug + "/" + entryPath
	url := baseURL + "/canvas/preview/" + previewPath + "?token=" + url.QueryEscape(token)
	writeJSON(w, http.StatusOK, canvasArtifactPreviewResponse{
		URL:            url,
		SandboxBaseURL: baseURL,
		Token:          token,
		ExpiresAt:      expiresAt.Format(time.RFC3339),
	})
}
