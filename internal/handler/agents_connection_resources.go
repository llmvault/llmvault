package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/mcp/catalog"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/tasks"
)

type updateAgentConnectionResourcesRequest struct {
	Resources map[string][]agentConnectionResourceSelection `json:"resources"`
}

type agentConnectionResourceSelection struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type,omitempty"`
	FullName string `json:"full_name,omitempty"`
}

type updateAgentConnectionResourcesResponse struct {
	AgentID      string     `json:"agent_id"`
	ConnectionID string     `json:"connection_id"`
	Resources    model.JSON `json:"resources"`
	CloneQueued  bool       `json:"clone_queued"`
}

// UpdateConnectionResources handles PUT /v1/agents/{id}/connections/{connectionID}/resources.
// @Summary Save selected connection resources for an agent
// @Description Saves provider resources such as selected GitHub repositories on the agent, then queues provider-specific reconciliation when needed.
// @Tags agents
// @Accept json
// @Produce json
// @Param id path string true "Agent UUID"
// @Param connectionID path string true "Connection UUID"
// @Param body body updateAgentConnectionResourcesRequest true "Selected resources grouped by resource type"
// @Success 200 {object} updateAgentConnectionResourcesResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/agents/{id}/connections/{connectionID}/resources [put]
func (h *AgentHandler) UpdateConnectionResources(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	org, ok := middleware.OrgFromContext(ctx)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing org context"})
		return
	}

	agentID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid agent id"})
		return
	}
	connectionID, err := uuid.Parse(chi.URLParam(r, "connectionID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid connection id"})
		return
	}

	var req updateAgentConnectionResourcesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}

	var agent model.Agent
	var conn model.Connection
	var normalized model.JSON
	err = h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.WithContext(ctx).
			Where("id = ? AND org_id = ? AND status <> ?", agentID, org.ID, "archived").
			First(&agent).Error; err != nil {
			return err
		}
		if err := tx.WithContext(ctx).Preload("Integration").
			Where("id = ? AND org_id = ? AND revoked_at IS NULL", connectionID, org.ID).
			First(&conn).Error; err != nil {
			return err
		}
		resources, err := h.normalizeConnectionResources(conn.Integration.Provider, req.Resources)
		if err != nil {
			return err
		}
		next := mergeAgentConnectionResources(agent.Resources, connectionID, resources)
		if err := tx.WithContext(ctx).Model(&model.Agent{}).
			Where("id = ? AND org_id = ?", agent.ID, org.ID).
			Update("resources", next).Error; err != nil {
			return fmt.Errorf("save agent connection resources: %w", err)
		}
		agent.Resources = next
		normalized = next
		return nil
	})
	if err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "agent or connection not found"})
		case strings.Contains(err.Error(), "not configurable"), strings.Contains(err.Error(), "missing"):
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		default:
			logging.CaptureWithFields(ctx, fmt.Errorf("update agent connection resources: %w", err), map[string]any{
				"org_id":        org.ID.String(),
				"agent_id":      agentID.String(),
				"connection_id": connectionID.String(),
			})
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to save resources"})
		}
		return
	}

	cloneQueued := h.enqueueConnectionResourceReconcile(ctx, agent, conn)
	writeJSON(w, http.StatusOK, updateAgentConnectionResourcesResponse{
		AgentID:      agent.ID.String(),
		ConnectionID: connectionID.String(),
		Resources:    normalized,
		CloneQueued:  cloneQueued,
	})
}

func (h *AgentHandler) normalizeConnectionResources(provider string, input map[string][]agentConnectionResourceSelection) (model.JSON, error) {
	out := model.JSON{}
	if len(input) == 0 {
		return out, nil
	}
	for resourceType, selections := range input {
		resourceType = strings.TrimSpace(resourceType)
		if resourceType == "" {
			return nil, fmt.Errorf("resource type is missing")
		}
		def, ok := catalog.Global().GetResourceDef(provider, resourceType)
		if !ok || !def.Configurable {
			return nil, fmt.Errorf("resource type %q is not configurable for provider %q", resourceType, provider)
		}
		items := make([]map[string]any, 0, len(selections))
		for _, selection := range selections {
			id := strings.TrimSpace(selection.ID)
			name := strings.TrimSpace(selection.Name)
			if id == "" || name == "" {
				return nil, fmt.Errorf("resource %q selection is missing id or name", resourceType)
			}
			item := map[string]any{
				"id":   id,
				"name": name,
				"type": resourceType,
			}
			fullName := strings.TrimSpace(selection.FullName)
			if fullName == "" && isGitHubProvider(provider) && resourceType == "repository" {
				fullName = id
			}
			if fullName != "" {
				item["full_name"] = fullName
			}
			items = append(items, item)
		}
		out[resourceType] = items
	}
	return out, nil
}

func mergeAgentConnectionResources(current model.JSON, connectionID uuid.UUID, resources model.JSON) model.JSON {
	next := model.JSON{}
	for key, value := range current {
		next[key] = value
	}
	key := connectionID.String()
	if len(resources) == 0 {
		delete(next, key)
		return next
	}
	next[key] = resources
	return next
}

func (h *AgentHandler) enqueueConnectionResourceReconcile(ctx context.Context, agent model.Agent, conn model.Connection) bool {
	if h == nil || h.enqueuer == nil || agent.OrgID == nil || !isGitHubProvider(conn.Integration.Provider) {
		return false
	}
	task, opts, err := tasks.NewAgentGitHubResourcesCloneTask(tasks.AgentGitHubResourcesClonePayload{
		OrgID:        *agent.OrgID,
		AgentID:      agent.ID,
		ConnectionID: conn.ID,
	})
	if err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("create github resource clone task: %w", err), map[string]any{
			"agent_id":      agent.ID.String(),
			"connection_id": conn.ID.String(),
		})
		return false
	}
	if _, err := h.enqueuer.EnqueueContext(ctx, task, opts...); err != nil {
		if errors.Is(err, asynq.ErrDuplicateTask) || errors.Is(err, asynq.ErrTaskIDConflict) {
			return true
		}
		logging.CaptureWithFields(ctx, fmt.Errorf("enqueue github resource clone task: %w", err), map[string]any{
			"agent_id":      agent.ID.String(),
			"connection_id": conn.ID.String(),
		})
		return false
	}
	return true
}

func isGitHubProvider(provider string) bool {
	provider = strings.TrimSpace(provider)
	return provider == "github-app" || provider == "github-app-oauth" || provider == "github" || provider == "github-pat"
}
