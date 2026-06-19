package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

// BuildEvents handles GET /v1/sandbox-templates/{id}/build-events.
// It streams a lightweight SSE view over the existing build status/log columns.
func (h *SandboxTemplateHandler) BuildEvents(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming is not supported"})
		return
	}
	id := chi.URLParam(r, "id")
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	var lastLogLen int
	var lastStatus string
	send := func(event string, payload any) {
		raw, _ := json.Marshal(payload)
		_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, raw)
		flusher.Flush()
	}

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		var tmpl model.SandboxTemplate
		err := h.db.WithContext(r.Context()).
			Select("id", "build_status", "build_error", "build_logs").
			Where("id = ? AND org_id = ?", id, org.ID).
			First(&tmpl).Error
		if err != nil {
			if err == gorm.ErrRecordNotFound {
				send("error", map[string]string{"error": "sandbox template not found"})
				return
			}
			send("error", map[string]string{"error": "failed to load sandbox template"})
			return
		}
		if tmpl.BuildStatus != lastStatus {
			lastStatus = tmpl.BuildStatus
			send("status", map[string]any{"build_status": tmpl.BuildStatus, "build_error": tmpl.BuildError})
		}
		if len(tmpl.BuildLogs) > lastLogLen {
			send("log", map[string]string{"chunk": tmpl.BuildLogs[lastLogLen:]})
			lastLogLen = len(tmpl.BuildLogs)
		}
		switch tmpl.BuildStatus {
		case "ready", "failed":
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}
