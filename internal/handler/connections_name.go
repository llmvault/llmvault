package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/access"
	"github.com/usehivy/hivy/internal/connectionname"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

type renameConnectionRequest struct {
	Name string `json:"name"`
}

// Rename updates the org-scoped name used for a connection's generated MCP server.
// @Summary Rename a connection
// @Tags connections
// @Accept json
// @Produce json
// @Param id path string true "Connection ID"
// @Param body body renameConnectionRequest true "Connection name"
// @Success 200 {object} connectionResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/connections/{id}/name [patch]
func (h *ConnectionHandler) Rename(w http.ResponseWriter, r *http.Request) {
	var req renameConnectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	identity, err := connectionname.Normalize(req.Name)
	if err != nil {
		writeConnectionNameError(w, err)
		return
	}
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing org context"})
		return
	}
	actor, err := access.Resolve(r.Context(), h.db, org.ID, middleware.UserID(r.Context()))
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "resolve connection rename actor", "error", err, "org_id", org.ID)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to resolve access"})
		return
	}
	if !actor.IsOrgManager() {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "not permitted"})
		return
	}
	connectionID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid connection id"})
		return
	}
	updates := map[string]any{"name": identity.Name, "slug": identity.Slug, "needs_name": false}
	result := h.db.WithContext(r.Context()).Model(&model.Connection{}).
		Where("id = ? AND org_id = ? AND revoked_at IS NULL", connectionID, org.ID).
		Updates(updates)
	if result.Error != nil {
		if isDuplicateKeyError(result.Error) {
			writeConnectionNameError(w, connectionname.ErrNameTaken)
			return
		}
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "rename connection", "error", result.Error, "connection_id", connectionID)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to rename connection"})
		return
	}
	if result.RowsAffected != 1 {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "connection not found"})
		return
	}
	var conn model.Connection
	if err := h.db.WithContext(r.Context()).Preload("Integration").
		Where("id = ? AND org_id = ? AND revoked_at IS NULL", connectionID, org.ID).
		First(&conn).Error; err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "load renamed connection", "error", err, "connection_id", connectionID)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load connection"})
		return
	}
	writeJSON(w, http.StatusOK, h.toConnectionResponse(conn))
}

func writeConnectionNameError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, connectionname.ErrInvalidName):
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "connection name must be between 1 and 80 characters and contain a letter or number"})
	case errors.Is(err, connectionname.ErrNameTaken), errors.Is(err, gorm.ErrDuplicatedKey):
		writeJSON(w, http.StatusConflict, errorResponse{Error: "connection name already exists"})
	default:
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to update connection name"})
	}
}
