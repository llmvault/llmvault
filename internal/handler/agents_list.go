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
	sandboxpkg "github.com/usehivy/hivy/internal/sandbox"
)

type agentSandboxSummary struct {
	ID             string  `json:"id"`
	Status         string  `json:"status"`
	ExternalID     string  `json:"external_id"`
	RuntimeVersion string  `json:"runtime_version,omitempty"`
	ErrorMessage   *string `json:"error_message,omitempty"`
	LastActiveAt   *string `json:"last_active_at,omitempty"`
	CreatedAt      string  `json:"created_at"`
	snapshotID     *string
}

type agentListItem struct {
	agentResponse
	UpgradeAvailable     bool                 `json:"upgrade_available"`
	LatestRuntimeVersion string               `json:"latest_runtime_version,omitempty"`
	Sandbox              *agentSandboxSummary `json:"sandbox,omitempty"`
}

// @Summary List AI agents
// @Description Returns all agents in the org with skills (metadata only — no bundle content),
// @Description triggers, and the latest sandbox row.
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
		Where("agents.org_id = ?", org.ID)

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
	sandboxes := h.loadMainAgentRuntimeSandboxSummaries(r.Context(), org.ID, agentIDs)
	channelIDs := h.loadAgentChannelIDs(r.Context(), org.ID, agentIDs)

	items := make([]agentListItem, len(agents))
	for i, a := range agents {
		base := toAgentResponse(a)
		base.ChannelIDs = channelIDs[a.ID]
		base.Triggers = triggers[a.ID]
		base.AttachedSkills = h.markAgentSkillLocks(r.Context(), org.ID, &a, skills[a.ID])
		currentSnapshotID := h.currentAgentSandboxSnapshotIDForAgent(a)
		items[i] = agentListItem{
			agentResponse:        base,
			UpgradeAvailable:     agentSandboxUpgradeAvailable(sandboxes[a.ID], currentSnapshotID),
			LatestRuntimeVersion: agentRuntimeVersionLabel(currentSnapshotID),
			Sandbox:              sandboxes[a.ID],
		}
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
// @Description triggers, and the latest sandbox row.
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
	sandbox := h.loadMainAgentRuntimeSandboxSummaries(r.Context(), org.ID, []uuid.UUID{agent.ID})[agent.ID]
	currentSnapshotID := h.currentAgentSandboxSnapshotIDForAgent(agent)

	writeJSON(w, http.StatusOK, agentListItem{
		agentResponse:        base,
		UpgradeAvailable:     agentSandboxUpgradeAvailable(sandbox, currentSnapshotID),
		LatestRuntimeVersion: agentRuntimeVersionLabel(currentSnapshotID),
		Sandbox:              sandbox,
	})
}

func (h *AgentHandler) currentAgentSandboxSnapshotID() string {
	return h.currentAgentSandboxSnapshotIDForSize(model.DefaultAgentSandboxSize)
}

func (h *AgentHandler) currentAgentSandboxSnapshotIDForSize(size string) string {
	if h == nil || h.compileDeps.Cfg == nil {
		return ""
	}
	return sandboxpkg.AgentRuntimeTemplateRefForSize(h.compileDeps.Cfg, size)
}

func (h *AgentHandler) currentAgentSandboxSnapshotIDForAgent(agent model.Agent) string {
	if h == nil || h.compileDeps.Cfg == nil {
		return ""
	}
	return sandboxpkg.AgentRuntimeTemplateRefForSandboxImageAndSize(h.compileDeps.Cfg, agent.SandboxImage, agent.SandboxSize)
}

func (h *AgentHandler) agentListItem(ctx context.Context, orgID uuid.UUID, agent model.Agent) agentListItem {
	base := toAgentResponse(agent)
	base.ChannelIDs = h.loadAgentChannelIDs(ctx, orgID, []uuid.UUID{agent.ID})[agent.ID]
	base.Triggers = h.loadAgentTriggers(agent.ID)[agent.ID]
	base.AttachedSkills = h.markAgentSkillLocks(ctx, orgID, &agent, h.loadAgentSkills(agent.ID)[agent.ID])
	sandbox := h.loadMainAgentRuntimeSandboxSummaries(ctx, orgID, []uuid.UUID{agent.ID})[agent.ID]
	currentSnapshotID := h.currentAgentSandboxSnapshotIDForAgent(agent)
	return agentListItem{
		agentResponse:        base,
		UpgradeAvailable:     agentSandboxUpgradeAvailable(sandbox, currentSnapshotID),
		LatestRuntimeVersion: agentRuntimeVersionLabel(currentSnapshotID),
		Sandbox:              sandbox,
	}
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

func agentSandboxUpgradeAvailable(summary *agentSandboxSummary, currentSnapshotID string) bool {
	if summary == nil {
		return false
	}
	if summary.snapshotID == nil || *summary.snapshotID == "" {
		return currentSnapshotID != ""
	}
	return *summary.snapshotID != currentSnapshotID
}

func (h *AgentHandler) loadMainAgentRuntimeSandboxSummaries(ctx context.Context, orgID uuid.UUID, agentIDs []uuid.UUID) map[uuid.UUID]*agentSandboxSummary {
	agentImage := ""
	if h != nil && h.compileDeps.Cfg != nil {
		agentImage = sandboxpkg.AgentRuntimeImageRef(h.compileDeps.Cfg, model.SandboxImageDefault)
	}
	return loadMainAgentRuntimeSandboxPerAgent(
		ctx,
		h.db,
		orgID,
		agentIDs,
		agentImage,
	)
}
