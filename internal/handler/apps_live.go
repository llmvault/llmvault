package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/usehivy/hivy/internal/sheets"
)

// appLiveHeartbeat is how often the live stream emits an SSE comment so idle
// connections (and any intervening proxy) stay open. A var so tests can shrink
// the interval without waiting the full production cadence.
var appLiveHeartbeat = 25 * time.Second

// Live streams the app's bound sheet's realtime events over SSE
// (GET /internal/apps/{appID}/v1/live). Auth is the app runtime secret
// (authApp), identical to the rest of the internal app API. The stream
// subscribes ONLY to the app's one sheet channel — an app can never observe
// another sheet's traffic. Each Redis event is relayed verbatim as
// `event: <event.type>` + `data: <event JSON>`; a `: ping` comment heartbeat
// keeps the connection warm.
func (h *AppsInternalHandler) Live(w http.ResponseWriter, r *http.Request) {
	app, ok := h.authApp(w, r)
	if !ok {
		return
	}
	if h.redis == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "live streaming is not configured"})
		return
	}

	ctx := r.Context()
	pubsub := h.redis.Subscribe(ctx, sheets.LiveChannel(app.SheetID))
	defer pubsub.Close()
	if _, err := pubsub.Receive(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "failed to subscribe to sheet stream"})
		return
	}
	liveCh := pubsub.Channel(redis.WithChannelSize(256))

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	rc := http.NewResponseController(w)
	// Open the stream immediately so the client sees a 200 before the first
	// event and any buffering proxy flushes headers.
	if err := writeAppLiveComment(rc, w, ": ok"); err != nil {
		return
	}

	heartbeat := time.NewTicker(appLiveHeartbeat)
	defer heartbeat.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case msg, chOk := <-liveCh:
			if !chOk {
				return
			}
			if err := writeAppLiveEvent(rc, w, msg.Payload); err != nil {
				return
			}
		case <-heartbeat.C:
			if err := writeAppLiveComment(rc, w, ": ping"); err != nil {
				return
			}
		}
	}
}

// writeAppLiveEvent relays one Redis event as an SSE frame named after the
// event's own type, with the original JSON as the data payload (verbatim — the
// app is the source of truth for parsing it).
func writeAppLiveEvent(rc *http.ResponseController, w http.ResponseWriter, payload string) error {
	name := "message"
	var meta struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal([]byte(payload), &meta); err == nil && meta.Type != "" {
		name = meta.Type
	}
	_ = rc.SetWriteDeadline(time.Now().Add(20 * time.Second))
	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, payload); err != nil {
		return err
	}
	return rc.Flush()
}

// writeAppLiveComment writes an SSE comment line (heartbeat / stream-open).
func writeAppLiveComment(rc *http.ResponseController, w http.ResponseWriter, comment string) error {
	_ = rc.SetWriteDeadline(time.Now().Add(20 * time.Second))
	if _, err := io.WriteString(w, comment+"\n\n"); err != nil {
		return err
	}
	return rc.Flush()
}
