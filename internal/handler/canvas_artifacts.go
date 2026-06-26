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

	"github.com/usehivy/hivy/internal/canvasartifact"
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

func (h *CanvasHandler) ListArtifacts(w http.ResponseWriter, r *http.Request) {
	org, ok := h.requireCanvasArtifactOrg(w, r)
	if !ok {
		return
	}
	filter, ok := parseCanvasArtifactFilter(w, r)
	if !ok {
		return
	}
	result, err := h.artifactSvc.ListArtifactsForOrg(r.Context(), org.ID, filter)
	if err != nil {
		logging.Capture(r.Context(), err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to list canvas artifacts"})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

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
	result, err := h.artifactSvc.GetArtifactForOrg(r.Context(), org.ID, artifactID)
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
	var artifact model.CanvasArtifact
	if err := h.db.WithContext(r.Context()).
		Preload("CanvasProject").
		Where("org_id = ? AND id = ? AND archived_at IS NULL", org.ID, artifactID).
		First(&artifact).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "canvas artifact not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load canvas artifact"})
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

func (h *CanvasHandler) ListAgentArtifacts(w http.ResponseWriter, r *http.Request) {
	if !h.requireCanvasArtifactRuntime(w) {
		return
	}
	agentID, ok := h.authorizeRuntimeRequest(w, r)
	if !ok {
		return
	}
	filter, ok := parseCanvasArtifactFilter(w, r)
	if !ok {
		return
	}
	result, err := h.artifactSvc.ListArtifactsForAgent(r.Context(), agentID, filter)
	if err != nil {
		h.writeRuntimeCanvasError(w, r, err, "list canvas artifacts", agentID)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *CanvasHandler) SnapshotAgentCanvas(w http.ResponseWriter, r *http.Request) {
	if !h.requireCanvasArtifactRuntime(w) {
		return
	}
	agentID, ok := h.authorizeRuntimeRequest(w, r)
	if !ok {
		return
	}
	result, err := h.artifactSvc.SnapshotForAgent(r.Context(), agentID)
	if err != nil {
		h.writeRuntimeCanvasError(w, r, err, "snapshot canvas artifacts", agentID)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *CanvasHandler) SyncAgentArtifact(w http.ResponseWriter, r *http.Request) {
	if !h.requireCanvasArtifactRuntime(w) {
		return
	}
	agentID, ok := h.authorizeRuntimeRequest(w, r)
	if !ok {
		return
	}
	var req canvasartifact.SyncRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	result, err := h.artifactSvc.SyncArtifactForAgent(r.Context(), agentID, req)
	if err != nil {
		h.writeRuntimeCanvasError(w, r, err, "sync canvas artifact", agentID)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
