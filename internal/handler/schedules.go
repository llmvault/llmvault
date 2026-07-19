package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/access"
	"github.com/usehivy/hivy/internal/agentschedule"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

// ScheduleHandler exposes CRUD over recurring agent schedules (cron/interval).
type ScheduleHandler struct {
	db *gorm.DB
}

func NewScheduleHandler(db *gorm.DB) *ScheduleHandler {
	return &ScheduleHandler{db: db}
}

type scheduleResponse struct {
	ID              string `json:"id"`
	Name            string `json:"name,omitempty"`
	AgentID         string `json:"agent_id"`
	AgentName       string `json:"agent_name"`
	AgentIcon       string `json:"agent_icon,omitempty"`
	Status          string `json:"status"`
	ScheduleKind    string `json:"schedule_kind"`
	CronExpression  string `json:"cron_expression,omitempty"`
	IntervalSeconds *int64 `json:"interval_seconds,omitempty"`
	RepeatCount     *int64 `json:"repeat_count,omitempty"`
	RepeatCompleted int64  `json:"repeat_completed"`
	Description     string `json:"description,omitempty"`
	TaskPrompt      string `json:"task_prompt,omitempty"`
	NextRunAt       string `json:"next_run_at,omitempty"`
	LastRunAt       string `json:"last_run_at,omitempty"`
	LastStatus      string `json:"last_status,omitempty"`
	LastError       string `json:"last_error,omitempty"`
	// LastRunSessionID links to the session of the most recent run, if any.
	LastRunSessionID string `json:"last_run_session_id,omitempty"`
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
}

type scheduleListResponse struct {
	Data []scheduleResponse `json:"data"`
}

type scheduleGetResponse struct {
	Schedule scheduleResponse `json:"schedule"`
}

type createScheduleRequest struct {
	Name            string `json:"name"`
	AgentID         string `json:"agent_id"`
	TaskPrompt      string `json:"task_prompt"`
	Description     string `json:"description"`
	CronExpression  string `json:"cron_expression"`
	IntervalSeconds *int64 `json:"interval_seconds"`
	RepeatCount     *int64 `json:"repeat_count"`
}

type updateScheduleRequest struct {
	Name            *string `json:"name,omitempty"`
	Status          *string `json:"status,omitempty"`
	Description     *string `json:"description,omitempty"`
	TaskPrompt      *string `json:"task_prompt,omitempty"`
	CronExpression  *string `json:"cron_expression,omitempty"`
	IntervalSeconds *int64  `json:"interval_seconds,omitempty"`
	RepeatCount     *int64  `json:"repeat_count,omitempty"`
}

type createScheduleResponse struct {
	Schedule scheduleResponse `json:"schedule"`
}

func scheduleToResponse(row model.AgentSchedule) scheduleResponse {
	resp := scheduleResponse{
		ID:              row.ID.String(),
		Name:            row.Name,
		AgentID:         row.AgentID.String(),
		AgentName:       fallbackAgentName(row.Agent),
		AgentIcon:       row.Agent.Icon,
		Status:          row.Status,
		ScheduleKind:    row.ScheduleKind,
		CronExpression:  row.CronExpression,
		IntervalSeconds: row.IntervalSeconds,
		RepeatCount:     row.RepeatCount,
		RepeatCompleted: row.RepeatCompleted,
		Description:     row.Description,
		TaskPrompt:      row.TaskPrompt,
		NextRunAt:       formatSchedulePtrTime(row.NextRunAt),
		LastRunAt:       formatSchedulePtrTime(row.LastRunAt),
		LastStatus:      row.LastStatus,
		LastError:       row.LastError,
		CreatedAt:       formatTriggerTime(row.CreatedAt),
		UpdatedAt:       formatTriggerTime(row.UpdatedAt),
	}
	return resp
}

func formatSchedulePtrTime(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

// actorCanAccessSchedule reports whether the actor may see/manage a schedule.
// Org managers see everything; otherwise the actor must belong to the
// scheduled agent's team.
func (h *ScheduleHandler) actorCanAccessSchedule(ctx context.Context, actor *access.Actor, schedule model.AgentSchedule) bool {
	if actor.IsOrgManager() {
		return true
	}
	var agent model.Agent
	if err := h.db.WithContext(ctx).Select("team_id").Where("id = ? AND org_id = ?", schedule.AgentID, schedule.OrgID).First(&agent).Error; err != nil {
		return false
	}
	allowed, err := actor.IsTeamMember(ctx, h.db, agent.TeamID)
	return err == nil && allowed
}

// List handles GET /v1/schedules.
// @Summary List schedules
// @Description Lists recurring agent schedules the caller can access.
// @Tags schedules
// @Produce json
// @Success 200 {object} scheduleListResponse
// @Failure 401 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/schedules [get]
func (h *ScheduleHandler) List(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok || org == nil {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing org context"})
		return
	}
	actor, err := access.Resolve(r.Context(), h.db, org.ID, middleware.UserID(r.Context()))
	if err != nil {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "forbidden"})
		return
	}
	rows, err := agentschedule.ListForOrg(r.Context(), h.db, org.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to list schedules"})
		return
	}
	data := make([]scheduleResponse, 0, len(rows))
	for i := range rows {
		if !h.actorCanAccessSchedule(r.Context(), actor, rows[i]) {
			continue
		}
		data = append(data, scheduleToResponse(rows[i]))
	}
	writeJSON(w, http.StatusOK, scheduleListResponse{Data: data})
}

// Get handles GET /v1/schedules/{id}.
// @Summary Get schedule
// @Description Gets one recurring agent schedule.
// @Tags schedules
// @Produce json
// @Param id path string true "Schedule ID"
// @Success 200 {object} scheduleGetResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/schedules/{id} [get]
func (h *ScheduleHandler) Get(w http.ResponseWriter, r *http.Request) {
	org, actor, id, ok := h.scheduleContext(w, r)
	if !ok {
		return
	}
	schedule, err := agentschedule.LoadByID(r.Context(), h.db, org.ID, id)
	if err != nil || !h.actorCanAccessSchedule(r.Context(), actor, *schedule) {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "schedule not found"})
		return
	}
	resp := scheduleToResponse(*schedule)
	resp.LastRunSessionID = h.lastRunSessionID(r.Context(), schedule.ID)
	writeJSON(w, http.StatusOK, scheduleGetResponse{Schedule: resp})
}

// lastRunSessionID returns the session id of the most recent run of a schedule
// that produced one, or "" if the schedule has never run in a session.
func (h *ScheduleHandler) lastRunSessionID(ctx context.Context, scheduleID uuid.UUID) string {
	var run model.AgentScheduleRun
	err := h.db.WithContext(ctx).
		Where("schedule_id = ? AND session_id IS NOT NULL", scheduleID).
		Order("created_at DESC").
		First(&run).Error
	if err != nil || run.SessionID == nil {
		return ""
	}
	return run.SessionID.String()
}

func (h *ScheduleHandler) scheduleContext(w http.ResponseWriter, r *http.Request) (*model.Org, *access.Actor, uuid.UUID, bool) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok || org == nil {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing org context"})
		return nil, nil, uuid.Nil, false
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil || id == uuid.Nil {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "schedule not found"})
		return nil, nil, uuid.Nil, false
	}
	actor, err := access.Resolve(r.Context(), h.db, org.ID, middleware.UserID(r.Context()))
	if err != nil {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "forbidden"})
		return nil, nil, uuid.Nil, false
	}
	return org, actor, id, true
}
