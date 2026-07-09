package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/tasks"
)

// Archive handles DELETE /v1/agents/{id}.
// @Summary Archive an agent
// @Description Archives an agent when it is not the default Hivy agent and has no active sessions.
// @Tags agents
// @Produce json
// @Param id path string true "Agent ID"
// @Success 200 {object} agentMutationResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/agents/{id} [delete]
func (h *AgentHandler) Archive(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logging.FromContext(ctx)
	org, ok := middleware.OrgFromContext(ctx)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing org context"})
		return
	}
	agentID, ok := agentIDFromRequest(w, r)
	if !ok {
		return
	}
	var agent model.Agent
	if err := h.db.WithContext(ctx).
		Where("id = ? AND org_id = ? AND status <> ?", agentID, org.ID, "archived").
		First(&agent).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "agent not found"})
			return
		}
		log.ErrorContext(ctx, "load agent for archive", "error", err, "agent_id", agentID, "org_id", org.ID)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load agent"})
		return
	}
	// Archiving an agent is a team-member action, gated on the actor being able to
	// manage the agent's team. This handler gate is the real authorization — the
	// route is member-reachable.
	if !h.authorizeAgentMutation(ctx, w, org.ID, &agent) {
		return
	}
	if agent.IsDefault {
		writeJSON(w, http.StatusConflict, errorResponse{Error: "the default Hivy agent cannot be deleted"})
		return
	}
	if err := h.archiveAgentWithSessions(ctx, org.ID, &agent); err != nil {
		switch {
		case errors.Is(err, errAgentBusy):
			writeJSON(w, http.StatusConflict, errorResponse{Error: "agent has sessions with an in-progress turn"})
		case errors.Is(err, errAgentChannelDefaultUnresolved):
			writeJSON(w, http.StatusConflict, errorResponse{Error: "this agent is a channel's default agent and no replacement Hivy agent could be found; re-point those channels first"})
		default:
			log.ErrorContext(ctx, "archive agent", "error", err, "agent_id", agent.ID, "org_id", org.ID)
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to archive agent"})
		}
		return
	}
	agent.Status = "archived"
	writeJSON(w, http.StatusOK, agentMutationResponse{Agent: h.agentListItem(ctx, org.ID, agent)})
}

// Sentinels returned by archiveAgentWithSessions so both DELETE /agents/{id} and
// catalog uninstall map them to a 409 instead of a generic 500.
var (
	errAgentBusy                     = errors.New("agent has sessions with an in-progress turn")
	errAgentChannelDefaultUnresolved = errors.New("agent is a channel default with no replacement Hivy")
)

// archiveAgentWithSessions is the ONE agent-archive routine, shared by
// Agent.Archive and catalog UninstallCatalog so both go through the same safety
// checks instead of diverging. It: (1) refuses when the agent has an in-progress
// turn; (2) re-points any channel whose default_agent_id is this agent onto that
// channel's team Hivy (the column is NOT NULL, so it cannot be cleared — leaving
// it would 404 resolveSessionAgent and brick the channel); (3) tears down the
// agent's open sessions and their sandboxes; (4) archives the agent and disables
// its triggers (re-homed onto a surviving default). Callers must reject
// IsDefault agents before calling.
func (h *AgentHandler) archiveAgentWithSessions(ctx context.Context, orgID uuid.UUID, agent *model.Agent) error {
	busy, err := h.agentHasBusySessions(ctx, orgID, agent.ID)
	if err != nil {
		return fmt.Errorf("check busy sessions: %w", err)
	}
	if busy {
		return errAgentBusy
	}
	if err := h.archiveAgentSessions(ctx, orgID, agent.ID); err != nil {
		return fmt.Errorf("archive agent sessions: %w", err)
	}
	return h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := repointChannelsFromArchivedAgent(tx, orgID, agent.ID); err != nil {
			return err
		}
		if err := tx.Model(&model.Agent{}).
			Where("id = ? AND org_id = ?", agent.ID, orgID).
			Update("status", "archived").Error; err != nil {
			return fmt.Errorf("archive agent: %w", err)
		}
		return reassignAgentTriggersToDefault(tx, orgID, []uuid.UUID{agent.ID})
	})
}

// repointChannelsFromArchivedAgent moves every non-archived channel whose default
// is agentID onto that channel's team Hivy (a team's undeletable default agent),
// falling back to an org-level default Hivy when the team has none. It returns
// errAgentChannelDefaultUnresolved when no replacement default agent exists, so
// the archive fails closed rather than bricking the channel.
func repointChannelsFromArchivedAgent(tx *gorm.DB, orgID, agentID uuid.UUID) error {
	var channels []model.Channel
	if err := tx.Where("org_id = ? AND default_agent_id = ? AND archived_at IS NULL", orgID, agentID).
		Find(&channels).Error; err != nil {
		return fmt.Errorf("load channels defaulting to agent: %w", err)
	}
	for i := range channels {
		ch := &channels[i]
		replacement, err := teamDefaultAgentID(tx, orgID, &ch.TeamID, agentID)
		if err != nil {
			return err
		}
		if replacement == uuid.Nil {
			return errAgentChannelDefaultUnresolved
		}
		if err := tx.Model(&model.Channel{}).
			Where("id = ? AND org_id = ?", ch.ID, orgID).
			Update("default_agent_id", replacement).Error; err != nil {
			return fmt.Errorf("re-point channel default agent: %w", err)
		}
	}
	return nil
}

// teamDefaultAgentID finds a channel's replacement default: the team's Hivy when
// teamID is set, else an org-level default Hivy. excludeID is the agent being
// archived. Returns uuid.Nil when none is found.
func teamDefaultAgentID(tx *gorm.DB, orgID uuid.UUID, teamID *uuid.UUID, excludeID uuid.UUID) (uuid.UUID, error) {
	q := tx.Model(&model.Agent{}).
		Where("org_id = ? AND is_default = ? AND status <> ? AND parent_agent_id IS NULL AND id <> ?",
			orgID, true, "archived", excludeID)
	if teamID != nil {
		q = q.Where("team_id = ?", *teamID)
	} else {
		q = q.Where("team_id IS NULL")
	}
	var replacement model.Agent
	err := q.Order("created_at ASC").First(&replacement).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// Team-scoped lookup missed (e.g. a team without its own Hivy); fall back
		// to any org default so the channel is never stranded.
		if teamID != nil {
			var any model.Agent
			if err2 := tx.Model(&model.Agent{}).
				Where("org_id = ? AND is_default = ? AND status <> ? AND parent_agent_id IS NULL AND id <> ?",
					orgID, true, "archived", excludeID).
				Order("created_at ASC").First(&any).Error; err2 == nil {
				return any.ID, nil
			}
		}
		return uuid.Nil, nil
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("load team default agent: %w", err)
	}
	return replacement.ID, nil
}

// agentHasBusySessions reports whether any of the agent's sessions currently has
// an in-progress agent turn. A merely-open but idle session does not block the
// archive; only a live turn does (ripping its sandbox out would abort the run).
func (h *AgentHandler) agentHasBusySessions(ctx context.Context, orgID, agentID uuid.UUID) (bool, error) {
	var busyCount int64
	if err := h.db.WithContext(ctx).
		Model(&model.Session{}).
		Where("org_id = ? AND agent_id = ? AND agent_turn_status = ?", orgID, agentID, model.SessionAgentTurnActive).
		Count(&busyCount).Error; err != nil {
		return false, err
	}
	return busyCount > 0, nil
}

// archiveAgentSessions archives every open (non-archived, non-ended) session for
// the agent and enqueues teardown of each session's sandbox. Enqueue failures
// are logged, not surfaced: the DB archive already succeeded and the sandbox
// reaper is the backstop.
func (h *AgentHandler) archiveAgentSessions(ctx context.Context, orgID, agentID uuid.UUID) error {
	var sessions []model.Session
	if err := h.db.WithContext(ctx).
		Where("org_id = ? AND agent_id = ? AND status = ?", orgID, agentID, "active").
		Find(&sessions).Error; err != nil {
		return err
	}
	now := time.Now()
	for i := range sessions {
		session := &sessions[i]
		updates := map[string]any{"status": "archived"}
		if session.EndedAt == nil {
			updates["ended_at"] = &now
		}
		if err := h.db.WithContext(ctx).
			Model(&model.Session{}).
			Where("id = ? AND org_id = ?", session.ID, orgID).
			Updates(updates).Error; err != nil {
			return err
		}
		if session.SandboxID == nil {
			continue
		}
		if err := tasks.EnqueueSandboxDelete(ctx, h.enqueuer, *session.SandboxID); err != nil {
			logging.CaptureWithFields(ctx, fmt.Errorf("enqueue sandbox delete on agent archive: %w", err), map[string]any{
				"agent_id":   agentID.String(),
				"session_id": session.ID.String(),
				"sandbox_id": session.SandboxID.String(),
			})
		}
	}
	return nil
}
