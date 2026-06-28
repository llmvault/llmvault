package handler

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

// List handles GET /v1/triggers.
// @Summary List triggers
// @Description Lists provider automation triggers installed in the current org.
// @Tags triggers
// @Produce json
// @Success 200 {object} triggerListResponse
// @Failure 401 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/triggers [get]
func (h *TriggerHandler) List(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok || org == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}
	var triggers []model.AgentTrigger
	if err := triggerQuery(h.db.WithContext(r.Context()), org.ID).
		Order("agent_triggers.created_at DESC, agent_triggers.id DESC").
		Find(&triggers).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to list triggers"})
		return
	}
	data := make([]triggerAutomationResponse, 0, len(triggers))
	for _, trigger := range triggers {
		data = append(data, triggerAutomationToResponse(trigger))
	}
	writeJSON(w, http.StatusOK, triggerListResponse{Data: data})
}

// Get handles GET /v1/triggers/{id}.
// @Summary Get trigger
// @Description Gets one provider automation trigger installed in the current org.
// @Tags triggers
// @Produce json
// @Param id path string true "Trigger ID"
// @Success 200 {object} triggerGetResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/triggers/{id} [get]
func (h *TriggerHandler) Get(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok || org == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}
	id, ok := triggerIDFromString(chi.URLParam(r, "id"))
	if !ok {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "trigger not found"})
		return
	}
	trigger, err := h.loadTrigger(r, org.ID, id)
	if err != nil {
		writeTriggerReadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, triggerGetResponse{Trigger: triggerAutomationToResponse(trigger)})
}

func (h *TriggerHandler) loadTrigger(r *http.Request, orgID, id uuid.UUID) (model.AgentTrigger, error) {
	var trigger model.AgentTrigger
	err := triggerQuery(h.db.WithContext(r.Context()), orgID).
		Where("agent_triggers.id = ?", id).
		First(&trigger).Error
	if err != nil {
		return model.AgentTrigger{}, err
	}
	return trigger, nil
}

func triggerQuery(db *gorm.DB, orgID uuid.UUID) *gorm.DB {
	return db.
		Model(&model.AgentTrigger{}).
		Joins("JOIN agents ON agents.id = agent_triggers.agent_id AND agents.status <> ?", "archived").
		Preload("Agent").
		Preload("Channel").
		Preload("Connection").
		Preload("Connection.Integration").
		Where("agent_triggers.org_id = ?", orgID)
}

func writeTriggerReadError(w http.ResponseWriter, err error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "trigger not found"})
		return
	}
	writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load trigger"})
}
