package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/channelagents"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

type agentListItem struct {
	agentResponse
}

// @Summary List AI agents
// @Description Returns the agents visible to the caller with skills (metadata only — no bundle content),
// @Description and triggers. Org managers and API-key callers see every agent in the org; a regular
// @Description member sees only agents assigned to channels they can use.
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

	ctx := r.Context()
	userID, _ := currentRequestUserID(ctx)
	orgRole, err := h.actorOrgRole(ctx, org.ID, userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load org membership"})
		return
	}

	q := h.db.WithContext(ctx).
		Preload("AgentCatalog").
		Where("agents.org_id = ?", org.ID).
		Where("agents.parent_agent_id IS NULL")

	// Managers and API-key callers see org-wide; a regular member sees only
	// agents assigned to channels they can use (mirrors channel visibility).
	if !isAPIKeyRequest(ctx) && !isOrgManager(orgRole) {
		q = q.Where("agents.id IN (?)", channelagents.VisibleAgentIDsSubquery(h.db, org.ID, userID))
	}

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
	skills := h.loadAgentSkills(ctx, agentIDs...)
	disabledByAgent, err := h.loadDisabledAgentPluginIDs(ctx, org.ID, agentIDs...)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load agent plugins"})
		return
	}

	items := make([]agentListItem, len(agents))
	for i, a := range agents {
		base := toAgentResponse(a)
		base.Triggers = triggers[a.ID]
		base.AttachedSkills = h.markAgentSkillLocks(r.Context(), org.ID, &a, skills[a.ID])
		base.DisabledPluginIDs = disabledByAgent[a.ID]
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
// @Description Returns one agent visible to the caller with skills (metadata only — no bundle content),
// @Description and triggers. A regular member can only fetch agents assigned to channels they can use;
// @Description an agent they cannot see returns 404.
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

	ctx := r.Context()
	userID, _ := currentRequestUserID(ctx)
	orgRole, err := h.actorOrgRole(ctx, org.ID, userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load org membership"})
		return
	}

	q := h.db.WithContext(ctx).
		Preload("AgentCatalog").
		Where("agents.id = ? AND agents.org_id = ? AND agents.status <> ?", agentID, org.ID, "archived")
	// A non-visible agent must be indistinguishable from a missing one (404,
	// not 403) so we never leak the existence of agents outside the caller's
	// channels. Managers and API-key callers keep org-wide access.
	if !isAPIKeyRequest(ctx) && !isOrgManager(orgRole) {
		q = q.Where("agents.id IN (?)", channelagents.VisibleAgentIDsSubquery(h.db, org.ID, userID))
	}

	var agent model.Agent
	if err := q.First(&agent).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get agent"})
		return
	}

	base := toAgentResponse(agent)
	disabledByAgent, err := h.loadDisabledAgentPluginIDs(ctx, org.ID, agent.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load agent plugins"})
		return
	}
	base.DisabledPluginIDs = disabledByAgent[agent.ID]
	base.Triggers = h.loadAgentTriggers(agent.ID)[agent.ID]
	base.AttachedSkills = h.markAgentSkillLocks(ctx, org.ID, &agent, h.loadAgentSkills(ctx, agent.ID)[agent.ID])
	base.SubAgents = h.loadSubAgentResponses(ctx, agent.ID)
	writeJSON(w, http.StatusOK, agentListItem{agentResponse: base})
}

// actorOrgRole returns the caller's org role, or "" when there is no user
// (API-key/proxy) or the user is not a member. It mirrors
// ChannelHandler.currentUserOrgRole so the agents surface applies the same
// manager check as channels without depending on the channel handler.
func (h *AgentHandler) actorOrgRole(ctx context.Context, orgID uuid.UUID, userID *uuid.UUID) (string, error) {
	if userID == nil {
		return "", nil
	}
	var membership model.OrgMembership
	err := h.db.WithContext(ctx).
		Where("org_id = ? AND user_id = ?", orgID, *userID).
		First(&membership).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	return membership.Role, err
}

func (h *AgentHandler) agentListItem(ctx context.Context, orgID uuid.UUID, agent model.Agent) (agentListItem, error) {
	base := toAgentResponse(agent)
	disabledByAgent, err := h.loadDisabledAgentPluginIDs(ctx, orgID, agent.ID)
	if err != nil {
		return agentListItem{}, err
	}
	base.DisabledPluginIDs = disabledByAgent[agent.ID]
	base.Triggers = h.loadAgentTriggers(agent.ID)[agent.ID]
	base.AttachedSkills = h.markAgentSkillLocks(ctx, orgID, &agent, h.loadAgentSkills(ctx, agent.ID)[agent.ID])
	return agentListItem{agentResponse: base}, nil
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
