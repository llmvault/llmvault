package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/canvasartifact"
	"github.com/usehivy/hivy/internal/logging"
)

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
