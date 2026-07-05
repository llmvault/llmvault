package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/access"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

// triggerResponse builds the API response, adding the webhook URL + secret state
// for HTTP triggers.
func (h *TriggerHandler) triggerResponse(trigger model.AgentTrigger) triggerAutomationResponse {
	resp := triggerAutomationToResponse(trigger)
	if trigger.TriggerType == "http" {
		resp.WebhookURL = h.httpTriggerWebhookURL(trigger.ID)
		resp.SecretSet = trigger.SecretKey != ""
	}
	return resp
}

// actorCanAccessTrigger reports whether the actor may see/manage a trigger.
// Provider (webhook) triggers keep their existing visibility; HTTP ("webhook")
// triggers are channel-scoped — the actor must be an org manager or able to use
// the trigger's channel.
func (h *TriggerHandler) actorCanAccessTrigger(ctx context.Context, actor *access.Actor, trigger model.AgentTrigger) bool {
	if trigger.TriggerType != "http" {
		return true
	}
	if actor.IsOrgManager() {
		return true
	}
	if trigger.Channel == nil {
		return false
	}
	return actor.CanUseChannel(ctx, h.db, *trigger.Channel)
}

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
	actor, err := access.Resolve(r.Context(), h.db, org.ID, middleware.UserID(r.Context()))
	if err != nil {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "forbidden"})
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
	for i := range triggers {
		if !h.actorCanAccessTrigger(r.Context(), actor, triggers[i]) {
			continue
		}
		data = append(data, h.triggerResponse(triggers[i]))
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
	actor, err := access.Resolve(r.Context(), h.db, org.ID, middleware.UserID(r.Context()))
	if err != nil || !h.actorCanAccessTrigger(r.Context(), actor, trigger) {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "trigger not found"})
		return
	}
	resp := h.triggerResponse(trigger)
	h.applyLastRun(r.Context(), &resp, trigger.ID)
	writeJSON(w, http.StatusOK, triggerGetResponse{Trigger: resp})
}

// applyLastRun fills the most-recent run time + session id for a trigger from
// agent_trigger_deliveries (every fire records one). Detail read only, so the
// UI can show a "last run" line that links to the session.
func (h *TriggerHandler) applyLastRun(ctx context.Context, resp *triggerAutomationResponse, triggerID uuid.UUID) {
	var delivery model.AgentTriggerDelivery
	err := h.db.WithContext(ctx).
		Where("trigger_id = ?", triggerID).
		Order("created_at DESC").
		First(&delivery).Error
	if err != nil {
		return
	}
	resp.LastRunAt = formatTriggerTime(delivery.CreatedAt)
	if delivery.SessionID != uuid.Nil {
		resp.LastRunSessionID = delivery.SessionID.String()
	}
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
