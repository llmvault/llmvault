package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

type employeeSessionEventResponse struct {
	ID                string          `json:"id"`
	EmployeeSessionID string          `json:"employee_session_id"`
	RuntimeSessionID  string          `json:"runtime_session_id"`
	EventID           string          `json:"event_id"`
	EventType         string          `json:"event_type"`
	Source            string          `json:"source"`
	Mode              string          `json:"mode"`
	SpecialistSlug    string          `json:"specialist_slug"`
	SpecialistTaskID  *string         `json:"specialist_task_id,omitempty"`
	SequenceNumber    int64           `json:"sequence_number"`
	Payload           json.RawMessage `json:"payload"`
	EventAt           string          `json:"event_at"`
	CreatedAt         string          `json:"created_at"`
}

// ListSessionEvents handles GET /v1/employees/{id}/sessions/{sessionID}/events.
// @Summary List persisted session events
// @Description Returns persisted employee session events ordered newest-first with cursor pagination.
// @Tags employees
// @Produce json
// @Param id path string true "Employee agent ID"
// @Param sessionID path string true "Employee session ID"
// @Param limit query int false "Page size"
// @Param cursor query string false "Pagination cursor"
// @Success 200 {object} paginatedResponse[employeeSessionEventResponse]
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/employees/{id}/sessions/{sessionID}/events [get]
func (h *EmployeeHandler) ListSessionEvents(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	org, ok := middleware.OrgFromContext(ctx)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}
	agentID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid employee id"})
		return
	}
	sessionID, err := uuid.Parse(chi.URLParam(r, "sessionID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid session id"})
		return
	}

	var session model.EmployeeSession
	if err := h.db.WithContext(ctx).
		Where("id = ? AND org_id = ? AND employee_id = ?", sessionID, org.ID, agentID).
		First(&session).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "employee session not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load employee session"})
		return
	}
	limit, cursor, err := parsePagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	query := h.db.WithContext(ctx).
		Where("org_id = ? AND employee_id = ? AND employee_session_id = ?", org.ID, agentID, sessionID)
	if cursor != nil {
		query = query.Where("(event_at, id) < (?, ?)", cursor.CreatedAt, cursor.ID)
	}
	query = query.Order("event_at DESC, id DESC").Limit(limit + 1)

	var events []model.EmployeeSessionEvent
	if err := query.Find(&events).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list employee session events"})
		return
	}
	hasMore := len(events) > limit
	if hasMore {
		events = events[:limit]
	}
	items := make([]employeeSessionEventResponse, len(events))
	for i, event := range events {
		items[i] = employeeSessionEventToResponse(event)
	}
	result := paginatedResponse[employeeSessionEventResponse]{Data: items, HasMore: hasMore}
	if hasMore {
		last := events[len(events)-1]
		c := encodeCursor(last.EventAt, last.ID)
		result.NextCursor = &c
	}
	writeJSON(w, http.StatusOK, result)
}

func employeeSessionEventToResponse(event model.EmployeeSessionEvent) employeeSessionEventResponse {
	var specialistTaskID *string
	if event.SpecialistTaskID != nil {
		value := event.SpecialistTaskID.String()
		specialistTaskID = &value
	}
	return employeeSessionEventResponse{
		ID:                event.ID.String(),
		EmployeeSessionID: event.EmployeeSessionID.String(),
		RuntimeSessionID:  event.SessionID,
		EventID:           event.EventID,
		EventType:         event.EventType,
		Source:            event.Source,
		Mode:              event.Mode,
		SpecialistSlug:    event.SpecialistSlug,
		SpecialistTaskID:  specialistTaskID,
		SequenceNumber:    event.SequenceNumber,
		Payload:           json.RawMessage(event.Payload),
		EventAt:           formatRuntimeTime(event.EventAt),
		CreatedAt:         formatRuntimeTime(event.CreatedAt),
	}
}
