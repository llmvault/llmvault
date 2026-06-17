package handler

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

// @Summary Disconnect an connection
// @Description Revokes a user's platform integration connection and removes it from Nango.
// @Tags connections
// @Produce json
// @Param id path string true "Connection ID"
// @Success 200 {object} map[string]string
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/connections/{id} [delete]
func (h *ConnectionHandler) Revoke(w http.ResponseWriter, r *http.Request) {
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
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "connection not found or already revoked"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to revoke connection"})
		return
	}
	nk := nangoProviderConfigKey(conn.Integration.UniqueKey)
	if err := h.nango.DeleteConnection(r.Context(), conn.NangoConnectionID, nk); err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "nango: delete connection failed, proceeding with local revocation",
			"error", err, "connection_id", connID, "nango_connection_id", conn.NangoConnectionID)
	}

	now := time.Now()
	result := h.db.Model(&model.Connection{}).
		Where("id = ? AND revoked_at IS NULL", connID).
		Update("revoked_at", &now)

	if result.Error != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to revoke connection"})
		return
	}
	if result.RowsAffected == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "connection not found or already revoked"})
		return
	}

	logging.FromContext(r.Context()).InfoContext(r.Context(), "connection revoked", "connection_id", conn.ID, "org_id", org.ID, "provider", conn.Integration.Provider)
	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}
