package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
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
		reportCanvasArtifactSyncDecodeError(r, agentID, err)
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

func reportCanvasArtifactSyncDecodeError(r *http.Request, agentID uuid.UUID, err error) {
	diagnostic := canvasArtifactSyncDecodeDiagnostic(err)
	requestContext := canvasArtifactSyncRequestContext(r)
	logging.FromContext(r.Context()).WarnContext(r.Context(), "canvas artifact sync request decode failed",
		"agent_id", agentID.String(),
		"error", err,
		"error_type", diagnostic["error_type"],
		"content_type", requestContext["content_type"],
		"content_length", requestContext["content_length"],
	)
	fields := map[string]any{
		"operation": "canvas_artifact_sync_decode",
		"agent_id":  agentID.String(),
		"decode":    diagnostic,
		"request":   requestContext,
	}
	if requestID := requestIDFromRequest(r); requestID != "" {
		fields["request_id"] = requestID
	}
	logging.CaptureWithFields(r.Context(), fmt.Errorf("canvas artifact sync decode request body: %w", err), fields)
}

func canvasArtifactSyncDecodeDiagnostic(err error) map[string]any {
	diagnostic := map[string]any{
		"error":      err.Error(),
		"error_type": fmt.Sprintf("%T", err),
	}
	var syntaxErr *json.SyntaxError
	if errors.As(err, &syntaxErr) {
		diagnostic["kind"] = "json_syntax"
		diagnostic["offset"] = syntaxErr.Offset
	}
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		diagnostic["kind"] = "json_type"
		diagnostic["field"] = typeErr.Field
		diagnostic["value"] = typeErr.Value
		diagnostic["offset"] = typeErr.Offset
		if typeErr.Type != nil {
			diagnostic["expected_type"] = typeErr.Type.String()
		}
	}
	return diagnostic
}

func canvasArtifactSyncRequestContext(r *http.Request) map[string]any {
	return map[string]any{
		"method":            r.Method,
		"path":              r.URL.Path,
		"content_type":      strings.TrimSpace(r.Header.Get("Content-Type")),
		"content_length":    r.ContentLength,
		"transfer_encoding": strings.Join(r.TransferEncoding, ","),
		"user_agent":        strings.TrimSpace(r.UserAgent()),
	}
}

func requestIDFromRequest(r *http.Request) string {
	if requestID := strings.TrimSpace(chimw.GetReqID(r.Context())); requestID != "" {
		return requestID
	}
	return strings.TrimSpace(r.Header.Get("X-Request-Id"))
}
