package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/access"
	"github.com/usehivy/hivy/internal/agentschedule"
	"github.com/usehivy/hivy/internal/channelagents"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

// Create handles POST /v1/schedules.
// @Summary Create schedule
// @Description Creates a recurring agent schedule (cron or interval).
// @Tags schedules
// @Accept json
// @Produce json
// @Param request body createScheduleRequest true "Schedule configuration"
// @Success 201 {object} createScheduleResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/schedules [post]
func (h *ScheduleHandler) Create(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok || org == nil {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing org context"})
		return
	}
	var req createScheduleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "name is required"})
		return
	}
	if strings.TrimSpace(req.TaskPrompt) == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "task_prompt is required"})
		return
	}
	agentID, err := uuid.Parse(strings.TrimSpace(req.AgentID))
	if err != nil || agentID == uuid.Nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "agent_id must be a uuid"})
		return
	}
	actor, err := access.Resolve(r.Context(), h.db, org.ID, middleware.UserID(r.Context()))
	if err != nil {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "forbidden"})
		return
	}
	if raw := strings.TrimSpace(req.ChannelID); raw != "" {
		channelID, perr := uuid.Parse(raw)
		if perr != nil || channelID == uuid.Nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "channel_id must be a uuid"})
			return
		}
		allowed, cerr := actor.CanUseChannelID(r.Context(), h.db, channelID)
		if cerr != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to check channel access"})
			return
		}
		if !allowed {
			writeJSON(w, http.StatusForbidden, errorResponse{Error: "you do not have access to this channel"})
			return
		}
		assigned, aerr := channelagents.Assigned(r.Context(), h.db, org.ID, channelID, agentID)
		if aerr != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to check channel agents"})
			return
		}
		if !assigned {
			writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: "agent is not assigned to this channel"})
			return
		}
	}

	var agent model.Agent
	if err := h.db.WithContext(r.Context()).
		Where("id = ? AND org_id = ?", agentID, org.ID).
		First(&agent).Error; err != nil {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "agent not found"})
		return
	}

	schedule, err := agentschedule.Create(r.Context(), h.db, &agent, agentschedule.CreateInput{
		Name:            strings.TrimSpace(req.Name),
		Description:     req.Description,
		TaskPrompt:      req.TaskPrompt,
		ChannelID:       req.ChannelID,
		CronExpression:  req.CronExpression,
		IntervalSeconds: req.IntervalSeconds,
		RepeatCount:     req.RepeatCount,
	})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	// Reload with the agent preloaded for the response.
	loaded, err := agentschedule.LoadByID(r.Context(), h.db, org.ID, schedule.ID)
	if err != nil {
		loaded = schedule
	}
	writeJSON(w, http.StatusCreated, createScheduleResponse{Schedule: scheduleToResponse(*loaded)})
}

// Update handles PATCH /v1/schedules/{id}.
// @Summary Update schedule
// @Description Updates a schedule's fields, or its status (active/paused/cancelled).
// @Tags schedules
// @Accept json
// @Produce json
// @Param id path string true "Schedule ID"
// @Param request body updateScheduleRequest true "Fields to update"
// @Success 200 {object} scheduleGetResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/schedules/{id} [patch]
func (h *ScheduleHandler) Update(w http.ResponseWriter, r *http.Request) {
	org, actor, id, ok := h.scheduleContext(w, r)
	if !ok {
		return
	}
	schedule, err := agentschedule.LoadByID(r.Context(), h.db, org.ID, id)
	if err != nil || !h.actorCanAccessSchedule(r.Context(), actor, *schedule) {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "schedule not found"})
		return
	}
	var req updateScheduleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	agent := schedule.Agent
	jobID := schedule.RuntimeJobID

	if req.ChannelID != nil {
		if raw := strings.TrimSpace(*req.ChannelID); raw != "" {
			channelID, perr := uuid.Parse(raw)
			if perr != nil || channelID == uuid.Nil {
				writeJSON(w, http.StatusBadRequest, errorResponse{Error: "channel_id must be a uuid"})
				return
			}
			allowed, cerr := actor.CanUseChannelID(r.Context(), h.db, channelID)
			if cerr != nil || !allowed {
				writeJSON(w, http.StatusForbidden, errorResponse{Error: "you do not have access to this channel"})
				return
			}
			assigned, aerr := channelagents.Assigned(r.Context(), h.db, org.ID, channelID, schedule.AgentID)
			if aerr != nil {
				writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to check channel agents"})
				return
			}
			if !assigned {
				writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: "agent is not assigned to this channel"})
				return
			}
		}
	}

	updated := schedule
	if req.Status != nil {
		updated, err = agentschedule.SetStatus(r.Context(), h.db, &agent, jobID, *req.Status)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
	}
	if hasScheduleFieldUpdate(req) {
		updated, err = agentschedule.Update(r.Context(), h.db, &agent, jobID, agentschedule.UpdateInput{
			Name:            req.Name,
			Description:     req.Description,
			TaskPrompt:      req.TaskPrompt,
			ChannelID:       req.ChannelID,
			CronExpression:  req.CronExpression,
			IntervalSeconds: req.IntervalSeconds,
			RepeatCount:     req.RepeatCount,
		})
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
	}
	loaded, err := agentschedule.LoadByID(r.Context(), h.db, org.ID, id)
	if err != nil {
		loaded = updated
	}
	writeJSON(w, http.StatusOK, scheduleGetResponse{Schedule: scheduleToResponse(*loaded)})
}

// Delete handles DELETE /v1/schedules/{id}. Cancels the schedule (soft delete);
// the runtime reconciler tears down the underlying job.
// @Summary Delete schedule
// @Description Cancels a recurring agent schedule.
// @Tags schedules
// @Produce json
// @Param id path string true "Schedule ID"
// @Success 200 {object} map[string]string
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/schedules/{id} [delete]
func (h *ScheduleHandler) Delete(w http.ResponseWriter, r *http.Request) {
	org, actor, id, ok := h.scheduleContext(w, r)
	if !ok {
		return
	}
	schedule, err := agentschedule.LoadByID(r.Context(), h.db, org.ID, id)
	if err != nil || !h.actorCanAccessSchedule(r.Context(), actor, *schedule) {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "schedule not found"})
		return
	}
	agent := schedule.Agent
	if _, err := agentschedule.SetStatus(r.Context(), h.db, &agent, schedule.RuntimeJobID, agentschedule.StatusCancelled); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to delete schedule"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func hasScheduleFieldUpdate(req updateScheduleRequest) bool {
	return req.Name != nil ||
		req.Description != nil ||
		req.TaskPrompt != nil ||
		req.ChannelID != nil ||
		req.CronExpression != nil ||
		req.IntervalSeconds != nil ||
		req.RepeatCount != nil
}
