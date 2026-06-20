package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	sandboxpkg "github.com/usehivy/hivy/internal/sandbox"
)

type PreviewActivityHandler struct {
	db           *gorm.DB
	orchestrator *sandboxpkg.Orchestrator
	token        string
}

type previewActivityRequest struct {
	Wake           bool `json:"wake"`
	TimeoutSeconds int  `json:"timeout_seconds,omitempty"`
}

func NewPreviewActivityHandler(db *gorm.DB, orchestrator *sandboxpkg.Orchestrator, token string) *PreviewActivityHandler {
	return &PreviewActivityHandler{db: db, orchestrator: orchestrator, token: strings.TrimSpace(token)}
}

func (h *PreviewActivityHandler) Handle(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.db == nil || strings.TrimSpace(h.token) == "" {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "preview activity disabled"})
		return
	}
	if !h.authorized(r) {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "unauthorized"})
		return
	}
	externalID := strings.TrimSpace(chi.URLParam(r, "externalID"))
	if externalID == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "external_id is required"})
		return
	}
	req := previewActivityRequest{}
	if r.Body != nil {
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON body"})
			return
		}
	}
	if parseBool(r.URL.Query().Get("wake")) {
		req.Wake = true
	}
	if timeout := strings.TrimSpace(r.URL.Query().Get("timeout_seconds")); timeout != "" {
		seconds, err := strconv.Atoi(timeout)
		if err != nil || seconds < 0 {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "timeout_seconds must be a positive integer"})
			return
		}
		req.TimeoutSeconds = seconds
	}

	now := time.Now().UTC()
	var sb model.Sandbox
	if err := h.db.WithContext(r.Context()).
		Where("provider_id = ? AND external_id = ?", sandboxpkg.ProviderMicrosandbox, externalID).
		First(&sb).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "sandbox not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load sandbox"})
		return
	}
	if err := h.db.WithContext(r.Context()).Model(&sb).Updates(map[string]any{
		"last_preview_at": now,
		"last_active_at":  now,
	}).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to record preview activity"})
		return
	}
	sb.LastPreviewAt = &now
	sb.LastActiveAt = &now

	writeJSON(w, http.StatusOK, map[string]any{
		"status":           sb.Status,
		"sandbox_id":       sb.ID.String(),
		"external_id":      sb.ExternalID,
		"runtime_url":      sb.RuntimeURL,
		"last_active_at":   now.Format(time.RFC3339),
		"last_preview_at":  now.Format(time.RFC3339),
		"managed_by_infra": req.Wake,
	})
}

func (h *PreviewActivityHandler) authorized(r *http.Request) bool {
	token := strings.TrimSpace(r.Header.Get("X-Hivy-Preview-Activity-Token"))
	if token == "" {
		auth := strings.TrimSpace(r.Header.Get("Authorization"))
		if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
			token = strings.TrimSpace(auth[len("bearer "):])
		}
	}
	return token != "" && token == h.token
}

func parseBool(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "y":
		return true
	default:
		return false
	}
}
