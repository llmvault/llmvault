package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

func (h *ConnectionHandler) Get(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	connID := chi.URLParam(r, "id")
	if connID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "connection id required"})
		return
	}

	var conn model.Connection
	if err := h.db.Preload("Integration").
		Where("id = ? AND org_id = ? AND revoked_at IS NULL", connID, org.ID).
		First(&conn).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "connection not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get connection"})
		return
	}
	writeJSON(w, http.StatusOK, h.toConnectionResponse(conn))
}
