package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/usehivy/hivy/internal/logging"
)

var sessionRuntimeStreamQueryKeys = []string{
	"replay",
	"after_seq",
	"from_turn_id",
	"follow",
}

// Stream handles GET /v1/sessions/{id}/stream.
// @Summary Stream a live agent session (SSE)
// @Description Proxies the private sandbox runtime's server-sent-events stream without routing live frames through Redis or Postgres.
// @Tags sessions
// @Produce text/event-stream
// @Param id path string true "Session ID"
// @Param replay query string false "Replay mode"
// @Param after_seq query integer false "Replay events after this runtime sequence"
// @Param from_turn_id query string false "Replay from this turn ID"
// @Param follow query boolean false "Continue following after replaying a turn"
// @Success 200 {string} string
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 502 {object} errorResponse
// @Failure 503 {object} errorResponse
// @Security BearerAuth
// @Router /v1/sessions/{id}/stream [get]
func (h *SessionHandler) Stream(w http.ResponseWriter, r *http.Request) {
	session, _, ok := h.authorizeSession(w, r, true)
	if !ok {
		return
	}
	sb, err := h.sessionSandboxForAccess(r.Context(), &session)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "load session sandbox for stream", "session_id", session.ID, "error", err)
		if errors.Is(err, errSessionSandboxUnavailable) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "session sandbox is not available yet"})
			return
		}
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "session stream is not available"})
		return
	}
	client, err := h.runtimeClientForSessionSandbox(r.Context(), sb)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "create runtime client for session stream", "session_id", session.ID, "sandbox_id", sb.ID, "error", err)
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "session stream is not available"})
		return
	}

	path := "/sessions/" + url.PathEscape(session.ID.String()) + "/stream"
	if query := sessionRuntimeStreamQuery(r).Encode(); query != "" {
		path += "?" + query
	}
	resp, err := client.StreamHTTP(r.Context(), path)
	if err != nil {
		if r.Context().Err() != nil {
			return
		}
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "open runtime session stream", "session_id", session.ID, "sandbox_id", sb.ID, "error", err)
		writeJSON(w, http.StatusBadGateway, errorResponse{Error: "failed to open session stream"})
		return
	}
	defer resp.Body.Close()
	if !strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "runtime session stream returned invalid content type", "session_id", session.ID, "sandbox_id", sb.ID, "content_type", resp.Header.Get("Content-Type"))
		writeJSON(w, http.StatusBadGateway, errorResponse{Error: "runtime returned an invalid session stream"})
		return
	}

	copySessionRuntimeStreamHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	rc := http.NewResponseController(w)
	if err := rc.Flush(); err != nil {
		return
	}
	_, err = io.CopyBuffer(&flushingSessionStreamWriter{w: w, rc: rc}, resp.Body, make([]byte, 32*1024))
	if err != nil && r.Context().Err() == nil {
		logging.FromContext(r.Context()).WarnContext(r.Context(), "relay runtime session stream", "session_id", session.ID, "sandbox_id", sb.ID, "error", err)
	}
}

func sessionRuntimeStreamQuery(r *http.Request) url.Values {
	query := make(url.Values)
	for _, key := range sessionRuntimeStreamQueryKeys {
		for _, value := range r.URL.Query()[key] {
			query.Add(key, value)
		}
	}
	if query.Get("after_seq") == "" {
		lastEventID := strings.TrimSpace(r.Header.Get("Last-Event-ID"))
		if sequence, err := strconv.ParseUint(lastEventID, 10, 64); err == nil {
			query.Set("after_seq", strconv.FormatUint(sequence, 10))
		}
	}
	return query
}

func copySessionRuntimeStreamHeaders(dst, src http.Header) {
	for _, key := range []string{
		"Content-Type",
		"Cache-Control",
		"X-Hivy-Stream-Id",
		"X-Hivy-Stream-Next-Sequence",
	} {
		if value := src.Get(key); value != "" {
			dst.Set(key, value)
		}
	}
	dst.Set("Cache-Control", "no-cache, no-transform")
	dst.Set("X-Accel-Buffering", "no")
}

type flushingSessionStreamWriter struct {
	w  http.ResponseWriter
	rc *http.ResponseController
}

func (w *flushingSessionStreamWriter) Write(raw []byte) (int, error) {
	_ = w.rc.SetWriteDeadline(time.Now().Add(20 * time.Second))
	n, err := w.w.Write(raw)
	if err != nil {
		return n, err
	}
	if err := w.rc.Flush(); err != nil {
		return n, err
	}
	return n, nil
}

func writeSessionSSE(rc *http.ResponseController, w http.ResponseWriter, eventName, id string, data any) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_ = rc.SetWriteDeadline(time.Now().Add(20 * time.Second))
	if eventName != "" {
		if _, err := fmt.Fprintf(w, "event: %s\n", eventName); err != nil {
			return err
		}
	}
	if id != "" {
		if _, err := fmt.Fprintf(w, "id: %s\n", id); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", raw); err != nil {
		return err
	}
	return rc.Flush()
}
