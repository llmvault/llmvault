package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

type agentSessionResponse struct {
	ID                    string                   `json:"id"`
	RuntimeConversationID string                   `json:"runtime_conversation_id"`
	Channel               string                   `json:"channel"`
	ThreadTS              string                   `json:"thread_ts"`
	AgentSessionID        string                   `json:"agent_session_id"`
	Source                string                   `json:"source"`
	SourceResourceKey     string                   `json:"source_resource_key"`
	Status                string                   `json:"status"`
	Name                  string                   `json:"name"`
	EventCount            int64                    `json:"event_count"`
	CreatedAt             string                   `json:"created_at"`
	UpdatedAt             string                   `json:"updated_at"`
	LastActivityAt        string                   `json:"last_activity_at"`
	EndedAt               *string                  `json:"ended_at,omitempty"`
	TriggerDelivery       *triggerDeliveryResponse `json:"trigger_delivery,omitempty"`
}

// ListSessions handles GET /v1/agents/{id}/sessions.
// @Summary List sessions for an agent
// @Description Returns persisted agent sessions with optional trigger delivery metadata.
// @Tags agents
// @Produce json
// @Param id path string true "Agent agent ID"
// @Param status query string false "Filter by status (active, completed, errored)"
// @Param session_id query string false "Exact session ID filter"
// @Param channel query string false "Exact channel filter"
// @Param thread_ts query string false "Exact thread timestamp filter"
// @Param agent_session_id query string false "Exact agent session ID filter"
// @Param q query string false "Search over session title, source key, or runtime conversation ID"
// @Param limit query int false "Page size"
// @Param cursor query string false "Pagination cursor"
// @Success 200 {object} paginatedResponse[agentSessionResponse]
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/agents/{id}/sessions [get]
func (h *AgentHandler) ListSessions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	org, ok := middleware.OrgFromContext(ctx)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	agentID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent id"})
		return
	}

	var agent model.Agent
	if err := h.db.WithContext(ctx).
		Where("id = ? AND org_id = ? AND status <> ?", agentID, org.ID, "archived").
		First(&agent).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load agent"})
		return
	}

	limit, cursor, err := parsePagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	query := h.db.WithContext(ctx).
		Where("org_id = ? AND agent_id = ?", org.ID, agentID)

	qp := r.URL.Query()
	if status := strings.TrimSpace(qp.Get("status")); status != "" {
		query = query.Where("status = ?", status)
	}
	if sessionID := strings.TrimSpace(qp.Get("session_id")); sessionID != "" {
		query = query.Where("runtime_conversation_id = ? OR id::text = ?", sessionID, sessionID)
	}
	if source := strings.TrimSpace(qp.Get("channel")); source != "" {
		query = query.Where("source = ?", source)
	}
	if threadTS := strings.TrimSpace(qp.Get("thread_ts")); threadTS != "" {
		query = query.Where("source_resource_key = ?", threadTS)
	}
	if agentSessionID := strings.TrimSpace(qp.Get("agent_session_id")); agentSessionID != "" {
		query = query.Where("runtime_conversation_id = ?", agentSessionID)
	}
	if search := strings.TrimSpace(qp.Get("q")); search != "" {
		like := "%" + strings.ToLower(search) + "%"
		query = query.Where(
			"LOWER(name) LIKE ? OR LOWER(source_resource_key) LIKE ? OR LOWER(runtime_conversation_id) LIKE ?",
			like, like, like,
		)
	}

	query = applyPagination(query, cursor, limit)
	var sessions []model.AgentSession
	if err := query.Find(&sessions).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list agent sessions"})
		return
	}

	hasMore := len(sessions) > limit
	if hasMore {
		sessions = sessions[:limit]
	}

	stats := h.loadAgentSessionStats(ctx, org.ID, agentID, sessionIDs(sessions))
	deliveries := h.loadTriggerDeliveriesForConversations(ctx, org.ID, agentID, sessionIDs(sessions))
	items := make([]agentSessionResponse, len(sessions))
	for i, session := range sessions {
		items[i] = agentSessionToResponse(session, stats[session.ID], deliveries[session.ID])
	}

	result := paginatedResponse[agentSessionResponse]{Data: items, HasMore: hasMore}
	if hasMore {
		last := sessions[len(sessions)-1]
		c := encodeCursor(last.CreatedAt, last.ID)
		result.NextCursor = &c
	}
	writeJSON(w, http.StatusOK, result)
}

type agentSessionStats struct {
	EventCount int64
	LastEvent  *time.Time
}

func sessionIDs(sessions []model.AgentSession) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(sessions))
	for _, session := range sessions {
		ids = append(ids, session.ID)
	}
	return ids
}

func (h *AgentHandler) loadAgentSessionStats(ctx context.Context, orgID, agentID uuid.UUID, sessionIDs []uuid.UUID) map[uuid.UUID]agentSessionStats {
	out := map[uuid.UUID]agentSessionStats{}
	if len(sessionIDs) == 0 {
		return out
	}
	type statRow struct {
		AgentSessionID uuid.UUID
		EventCount     int64
		LastEvent      *time.Time
	}
	var rows []statRow
	if err := h.db.WithContext(ctx).
		Model(&model.AgentSessionEvent{}).
		Select("agent_session_id, COUNT(*) AS event_count, MAX(event_at) AS last_event").
		Where("org_id = ? AND agent_id = ? AND agent_session_id IN ?", orgID, agentID, sessionIDs).
		Group("agent_session_id").
		Scan(&rows).Error; err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "load agent session stats", "error", err, "agent_id", agentID)
		return out
	}
	for _, row := range rows {
		out[row.AgentSessionID] = agentSessionStats{EventCount: row.EventCount, LastEvent: row.LastEvent}
	}
	return out
}

func (h *AgentHandler) loadTriggerDeliveriesForConversations(ctx context.Context, orgID, agentID uuid.UUID, sessionIDs []uuid.UUID) map[uuid.UUID]*triggerDeliveryResponse {
	out := map[uuid.UUID]*triggerDeliveryResponse{}
	if len(sessionIDs) == 0 {
		return out
	}
	var rows []model.AgentTriggerDelivery
	if err := h.db.WithContext(ctx).
		Where("org_id = ? AND agent_id = ? AND conversation_id IN ?", orgID, agentID, sessionIDs).
		Order("created_at DESC").
		Find(&rows).Error; err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "load trigger deliveries for agent sessions", "error", err, "agent_id", agentID)
		return out
	}
	for _, row := range rows {
		if _, exists := out[row.ConversationID]; exists {
			continue
		}
		resp := triggerDeliveryToResponse(row)
		out[row.ConversationID] = &resp
	}
	return out
}

func agentSessionToResponse(session model.AgentSession, stats agentSessionStats, delivery *triggerDeliveryResponse) agentSessionResponse {
	lastActivity := session.UpdatedAt
	if stats.LastEvent != nil && !stats.LastEvent.IsZero() {
		lastActivity = *stats.LastEvent
	}
	endedAt := formatRuntimeTimePtr(session.EndedAt)
	return agentSessionResponse{
		ID:                    session.ID.String(),
		RuntimeConversationID: session.RuntimeConversationID,
		Channel:               session.Source,
		ThreadTS:              session.SourceResourceKey,
		AgentSessionID:        session.RuntimeConversationID,
		Source:                session.Source,
		SourceResourceKey:     session.SourceResourceKey,
		Status:                session.Status,
		Name:                  session.Name,
		EventCount:            stats.EventCount,
		CreatedAt:             formatRuntimeTime(session.CreatedAt),
		UpdatedAt:             formatRuntimeTime(session.UpdatedAt),
		LastActivityAt:        formatRuntimeTime(lastActivity),
		EndedAt:               endedAt,
		TriggerDelivery:       delivery,
	}
}

func formatRuntimeTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

func formatRuntimeTimePtr(value *time.Time) *string {
	if value == nil || value.IsZero() {
		return nil
	}
	formatted := formatRuntimeTime(*value)
	return &formatted
}
