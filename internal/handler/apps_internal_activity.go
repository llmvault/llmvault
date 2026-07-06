package handler

import (
	"net/http"
	"time"

	"github.com/usehivy/hivy/internal/model"
)

// Activity refreshes the app sandbox's idle timer. hivycore posts it (debounced,
// prod only) on served requests so auto-sleep can stop an app after a window of
// no real visits; it wakes on the next request. A not-yet-deployed app is a no-op.
func (h *AppsInternalHandler) Activity(w http.ResponseWriter, r *http.Request) {
	app, ok := h.authApp(w, r)
	if !ok {
		return
	}
	if app.SandboxID == nil {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}
	if err := h.db.WithContext(r.Context()).Model(&model.Sandbox{}).
		Where("id = ?", *app.SandboxID).
		Update("last_app_activity_at", time.Now().UTC()).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to record activity"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
