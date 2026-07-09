package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/usehivy/hivy/internal/runtimestream"
)

// @Summary Stream session notices (SSE)
// @Description Server-sent-events stream that forwards typed notices scoped to the session. Content-Type is text/event-stream.
// @Tags sessions
// @Produce text/event-stream
// @Param id path string true "Session ID"
// @Success 200 {object} map[string]interface{}
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/sessions/{id}/notices [get]
func (h *SessionHandler) Notices(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.runtimeStreamStore == nil || h.runtimeStreamStore.Redis() == nil {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "session stream is not configured"})
		return
	}
	session, _, ok := h.authorizeSession(w, r, false)
	if !ok {
		return
	}
	ctx := r.Context()
	pubsub := h.runtimeStreamStore.Redis().Subscribe(ctx,
		runtimestream.LiveChannel(session.ID.String()),
	)
	defer pubsub.Close()
	if _, err := pubsub.Receive(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "failed to subscribe to session notices"})
		return
	}
	liveCh := pubsub.Channel(redis.WithChannelSize(1024))

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	rc := http.NewResponseController(w)
	_ = rc.Flush()

	keepalive := time.NewTicker(15 * time.Second)
	defer keepalive.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-liveCh:
			if !ok {
				return
			}
			if err := emitSessionNotice(rc, w, msg.Payload); err != nil {
				return
			}
		case <-keepalive.C:
			if err := writeSessionSSE(rc, w, "session.control", "", map[string]any{
				"type":      "keepalive",
				"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
			}); err != nil {
				return
			}
		}
	}
}

func emitSessionNotice(rc *http.ResponseController, w http.ResponseWriter, raw string) error {
	var msg runtimestream.LiveMessage
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		return nil
	}
	if msg.Kind != runtimestream.LiveKindNotice || msg.Notice == nil {
		return nil
	}
	return writeSessionSSE(rc, w, "session.notice", "", msg.Notice)
}
