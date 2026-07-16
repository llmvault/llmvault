package handler

import (
	"encoding/json"
	"net/http"

	"github.com/usehivy/hivy/internal/access"
	"github.com/usehivy/hivy/internal/connectionname"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

// Rename updates the org-scoped name used for a database connection's MCP server.
// @Summary Rename a database integration
// @Tags database-integrations
// @Accept json
// @Produce json
// @Param id path string true "Database integration ID"
// @Param body body renameConnectionRequest true "Connection name"
// @Success 200 {object} databaseConnectionResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/database-integrations/{id}/name [patch]
func (h *DatabaseIntegrationHandler) Rename(w http.ResponseWriter, r *http.Request) {
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
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "resolve database connection rename actor", "error", err, "org_id", org.ID)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to resolve access"})
		return
	}
	if !actor.IsOrgManager() {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "not permitted"})
		return
	}
	conn, ok := h.loadOrgConnection(w, r)
	if !ok {
		return
	}
	result := h.db.WithContext(r.Context()).Model(&model.DatabaseConnection{}).
		Where("id = ? AND org_id = ? AND revoked_at IS NULL", conn.ID, org.ID).
		Updates(map[string]any{"name": identity.Name, "slug": identity.Slug, "needs_name": false})
	if result.Error != nil {
		if isDuplicateKeyError(result.Error) {
			writeConnectionNameError(w, connectionname.ErrNameTaken)
			return
		}
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "rename database connection", "error", result.Error, "connection_id", conn.ID)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to rename database connection"})
		return
	}
	if result.RowsAffected != 1 {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "database integration not found"})
		return
	}
	conn.Name = identity.Name
	conn.Slug = identity.Slug
	conn.NeedsName = false
	writeJSON(w, http.StatusOK, databaseConnectionToResponse(conn))
}
