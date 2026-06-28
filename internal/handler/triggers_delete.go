package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

// Delete handles DELETE /v1/triggers/{id}.
// @Summary Delete trigger
// @Description Deletes a provider automation trigger installed in the current org.
// @Tags triggers
// @Produce json
// @Param id path string true "Trigger ID"
// @Success 200 {object} map[string]string
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/triggers/{id} [delete]
func (h *TriggerHandler) Delete(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok || org == nil {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing org context"})
		return
	}
	id, ok := triggerIDFromString(chi.URLParam(r, "id"))
	if !ok {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "trigger not found"})
		return
	}
	result := h.db.WithContext(r.Context()).
		Where("id = ? AND org_id = ?", id, org.ID).
		Delete(&model.AgentTrigger{})
	if result.Error != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to delete trigger"})
		return
	}
	if result.RowsAffected == 0 {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "trigger not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
