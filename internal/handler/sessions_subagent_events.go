package handler

import (
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

// ListSubagentEvents handles GET /v1/sessions/{id}/subagents/{childSessionID}/events.
// @Summary List subagent session events
// @Description Lists transcript events for one child session of an authorized parent session.
// @Tags sessions
// @Produce json
// @Param id path string true "Parent session ID"
// @Param childSessionID path string true "Child session ID"
// @Param limit query int false "Page size"
// @Param cursor query string false "Pagination cursor"
// @Success 200 {object} paginatedResponse[sessionEventResponse]
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/sessions/{id}/subagents/{childSessionID}/events [get]
func (h *SessionHandler) ListSubagentEvents(w http.ResponseWriter, r *http.Request) {
	session, _, ok := h.authorizeSession(w, r, false)
	if !ok {
		return
	}
	childSessionID, ok := normalizeChildSessionID(chi.URLParam(r, "childSessionID"))
	if !ok {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid child session id"})
		return
	}
	limit, cursor, err := parsePagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}

	query := h.db.WithContext(r.Context()).
		Where("org_id = ? AND session_id = ?", session.OrgID, session.ID).
		Where("payload->>'scope' = ?", "subagent").
		Where("payload->'subagent'->>'child_session_id' = ?", childSessionID)
	query = applyPagination(query, cursor, limit)
	var events []model.SessionEvent
	if err := query.Find(&events).Error; err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "list subagent events", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to list subagent events"})
		return
	}

	hasMore := len(events) > limit
	if hasMore {
		events = events[:limit]
	}
	out := make([]sessionEventResponse, len(events))
	for i, event := range events {
		out[i] = eventToResponse(event)
	}
	resp := paginatedResponse[sessionEventResponse]{Data: out, HasMore: hasMore}
	if hasMore {
		last := events[len(events)-1]
		next := encodeCursor(last.CreatedAt, last.ID)
		resp.NextCursor = &next
	}
	writeJSON(w, http.StatusOK, resp)
}

func normalizeChildSessionID(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 255 || !utf8.ValidString(value) {
		return "", false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return "", false
		}
	}
	return value, true
}
