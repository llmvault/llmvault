package handler

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/model"
)

// StreamAccess handles GET /v1/sessions/{id}/stream-access.
// @Summary Get direct session stream access
// @Description Returns browser-direct sandbox stream details for a delivered queued session message.
// @Tags sessions
// @Produce json
// @Param id path string true "Session ID"
// @Param event_id query string false "Session event ID"
// @Success 200 {object} sessionStreamAccessResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 503 {object} errorResponse
// @Security BearerAuth
// @Router /v1/sessions/{id}/stream-access [get]
func (h *SessionHandler) StreamAccess(w http.ResponseWriter, r *http.Request) {
	session, _, ok := h.authorizeSession(w, r, true)
	if !ok {
		return
	}
	if h.runtimeEncKey == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "runtime stream access is not configured"})
		return
	}
	queue, ok := h.loadStreamQueue(w, r, session.ID)
	if !ok {
		return
	}
	if strings.TrimSpace(queue.RuntimeStreamID) == "" || strings.TrimSpace(queue.RuntimeStreamURL) == "" {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "runtime stream is not available yet"})
		return
	}
	if session.SandboxID == nil {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "session sandbox is not available yet"})
		return
	}
	var sb model.Sandbox
	if err := h.db.WithContext(r.Context()).
		Where("id = ? AND org_id = ?", *session.SandboxID, session.OrgID).
		First(&sb).Error; err != nil {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "session sandbox is not available yet"})
		return
	}
	runtimeSecret, err := h.runtimeEncKey.DecryptString(sb.EncryptedRuntimeSecret)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "runtime stream access is not available"})
		return
	}
	streamURL := strings.TrimSpace(queue.RuntimeStreamURL)
	writeJSON(w, http.StatusOK, sessionStreamAccessResponse{
		SessionID:      session.ID.String(),
		SessionEventID: queue.SessionEventID.String(),
		SequenceNumber: queue.SequenceNumber,
		StreamID:       strings.TrimSpace(queue.RuntimeStreamID),
		StreamURL:      streamURL,
		DirectURL:      directRuntimeStreamURL(sb.RuntimeURL, streamURL),
		StreamToken:    agentruntime.StreamTokenFromRuntimeSecret(runtimeSecret),
		TraceID:        strings.TrimSpace(queue.RuntimeTraceID),
		TurnID:         strings.TrimSpace(queue.RuntimeTurnID),
	})
}

func (h *SessionHandler) loadStreamQueue(w http.ResponseWriter, r *http.Request, sessionID uuid.UUID) (model.SessionMessageQueue, bool) {
	query := h.db.WithContext(r.Context()).
		Where("session_id = ? AND status = ?", sessionID, "delivered")
	if rawEventID := strings.TrimSpace(r.URL.Query().Get("event_id")); rawEventID != "" {
		eventID, err := uuid.Parse(rawEventID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid event_id"})
			return model.SessionMessageQueue{}, false
		}
		query = query.Where("session_event_id = ?", eventID)
	} else {
		query = query.Where("runtime_stream_id <> '' AND runtime_stream_url <> ''").
			Order("sequence_number DESC")
	}
	var queue model.SessionMessageQueue
	if err := query.First(&queue).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "runtime stream is not available yet"})
			return model.SessionMessageQueue{}, false
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load runtime stream"})
		return model.SessionMessageQueue{}, false
	}
	return queue, true
}

func directRuntimeStreamURL(runtimeURL, streamURL string) string {
	base := strings.TrimRight(strings.TrimSpace(runtimeURL), "/")
	path := strings.TrimSpace(streamURL)
	if base == "" || path == "" {
		return ""
	}
	parsed, err := url.Parse(path)
	if err == nil && parsed.IsAbs() {
		return path
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return base + path
}
