package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

type agentListItem struct {
	agentResponse
}

// @Summary List AI agents
// @Description Returns all agents in the org with skills (metadata only — no bundle content),
// @Description and triggers.
// @Tags agents
// @Produce json
// @Param status query string false "Filter by status (draft, active, archived)"
// @Param limit query int false "Page size (default 50, max 100)"
// @Param cursor query string false "Pagination cursor"
// @Success 200 {object} paginatedResponse[agentListItem]
// @Failure 401 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/agents [get]
func (h *AgentHandler) List(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	limit, cursor, err := parsePagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	q := h.db.WithContext(r.Context()).
		Preload("AgentCatalog").
		Where("agents.org_id = ?", org.ID).
		Where("agents.parent_agent_id IS NULL")

	if status := r.URL.Query().Get("status"); status != "" {
		q = q.Where("agents.status = ?", status)
	}

	q = applyPagination(q, cursor, limit)

	var agents []model.Agent
	if err := q.Find(&agents).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list agents"})
		return
	}

	hasMore := len(agents) > limit
	if hasMore {
		agents = agents[:limit]
	}

	agentIDs := make([]uuid.UUID, len(agents))
	for i, a := range agents {
		agentIDs[i] = a.ID
	}

	triggers := h.loadAgentTriggers(agentIDs...)
	skills := h.loadAgentSkills(agentIDs...)
	channelIDs := h.loadAgentChannelIDs(r.Context(), org.ID, agentIDs)

	items := make([]agentListItem, len(agents))
	for i, a := range agents {
		base := toAgentResponse(a)
		base.ChannelIDs = channelIDs[a.ID]
		base.Triggers = triggers[a.ID]
		base.AttachedSkills = h.markAgentSkillLocks(r.Context(), org.ID, &a, skills[a.ID])
		items[i] = agentListItem{agentResponse: base}
	}

	result := paginatedResponse[agentListItem]{Data: items, HasMore: hasMore}
	if hasMore {
		last := agents[len(agents)-1]
		c := encodeCursor(last.CreatedAt, last.ID)
		result.NextCursor = &c
	}
	writeJSON(w, http.StatusOK, result)
}

// Get handles GET /v1/agents/{id}.
// @Summary Get an AI agent
// @Description Returns one agent in the org with skills (metadata only — no bundle content),
// @Description and triggers.
// @Tags agents
// @Produce json
// @Param id path string true "Agent agent ID"
// @Success 200 {object} agentListItem
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/agents/{id} [get]
func (h *AgentHandler) Get(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
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
	if err := h.db.WithContext(r.Context()).
		Preload("AgentCatalog").
		Where("agents.id = ? AND agents.org_id = ? AND agents.status <> ?", agentID, org.ID, "archived").
		First(&agent).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get agent"})
		return
	}

	base := toAgentResponse(agent)
	base.ChannelIDs = h.loadAgentChannelIDs(r.Context(), org.ID, []uuid.UUID{agent.ID})[agent.ID]
	base.Triggers = h.loadAgentTriggers(agent.ID)[agent.ID]
	base.AttachedSkills = h.markAgentSkillLocks(r.Context(), org.ID, &agent, h.loadAgentSkills(agent.ID)[agent.ID])
	base.SubAgents = h.loadSubAgentResponses(r.Context(), agent.ID)
	writeJSON(w, http.StatusOK, agentListItem{agentResponse: base})
}

func (h *AgentHandler) agentListItem(ctx context.Context, orgID uuid.UUID, agent model.Agent) agentListItem {
	base := toAgentResponse(agent)
	base.ChannelIDs = h.loadAgentChannelIDs(ctx, orgID, []uuid.UUID{agent.ID})[agent.ID]
	base.Triggers = h.loadAgentTriggers(agent.ID)[agent.ID]
	base.AttachedSkills = h.markAgentSkillLocks(ctx, orgID, &agent, h.loadAgentSkills(agent.ID)[agent.ID])
	return agentListItem{agentResponse: base}
}

func (h *AgentHandler) markAgentSkillLocks(ctx context.Context, orgID uuid.UUID, agent *model.Agent, skills []agentSkillSummary) []agentSkillSummary {
	if len(skills) == 0 {
		return skills
	}
	locked, err := agentLockedSkillIDs(ctx, h.db, orgID, agent)
	if err != nil || len(locked) == 0 {
		return skills
	}
	for index := range skills {
		id, err := uuid.Parse(skills[index].ID)
		if err == nil && locked[id] {
			skills[index].Locked = true
			skills[index].Required = true
		}
	}
	return skills
}
