package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/credentials"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/registry"
)

type updateAgentModelRequest struct {
	Model string `json:"model"`
}

type updateAgentModelResponse struct {
	Agent agentResponse `json:"agent"`
}

// ListModels handles GET /v1/agents/models.
// @Summary List agent-selectable models
// @Description Returns canonical models backed by active org or system credentials.
// @Tags agents
// @Produce json
// @Success 200 {array} modelSummary
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/agents/models [get]
func (h *AgentHandler) ListModels(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing org context"})
		return
	}
	models, err := h.agentModelSummaries(r.Context(), org.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load agent models"})
		return
	}
	writeJSON(w, http.StatusOK, models)
}

// UpdateModel handles PATCH /v1/agents/{id}/model.
// @Summary Update an agent model
// @Description Persists the agent's default model. New runtime messages use the saved model through the session delivery path.
// @Tags agents
// @Accept json
// @Produce json
// @Param id path string true "Agent ID"
// @Param body body updateAgentModelRequest true "Agent model update"
// @Success 200 {object} updateAgentModelResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Failure 502 {object} errorResponse
// @Security BearerAuth
// @Router /v1/agents/{id}/model [patch]
func (h *AgentHandler) UpdateModel(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logging.FromContext(ctx)

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

	var req updateAgentModelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	modelID := strings.TrimSpace(req.Model)

	var agent model.Agent
	if err := h.db.WithContext(ctx).
		Preload("AgentCatalog").
		Where("id = ? AND org_id = ? AND status <> ?", agentID, org.ID, "archived").
		First(&agent).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "agent not found"})
			return
		}
		log.ErrorContext(ctx, "load agent for model update", "error", err, "agent_id", agentID, "org_id", org.ID)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load agent"})
		return
	}
	// Changing the model is a team-member action, gated on the actor being able
	// to manage the agent's team. This handler gate is the real authorization —
	// the route is member-reachable (mirrors Update).
	if !h.authorizeAgentMutation(ctx, w, org.ID, &agent) {
		return
	}
	if err := h.validateAgentSelectableModel(ctx, org.ID, modelID); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}

	if agent.Model != modelID {
		if err := h.db.WithContext(ctx).
			Model(&model.Agent{}).
			Where("id = ? AND org_id = ?", agent.ID, org.ID).
			Update("model", modelID).Error; err != nil {
			log.ErrorContext(ctx, "update agent model", "error", err, "agent_id", agent.ID, "org_id", org.ID)
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to update agent model"})
			return
		}
		agent.Model = modelID
	}

	log.InfoContext(ctx, "agent model updated",
		"agent_id", agent.ID,
		"model", modelID,
	)
	writeJSON(w, http.StatusOK, updateAgentModelResponse{
		Agent: toAgentResponse(agent),
	})
}

func (h *AgentHandler) agentModelRegistry() *registry.Registry {
	if h != nil && h.registry != nil {
		return h.registry
	}
	return registry.Global()
}

func (h *AgentHandler) agentModelSummaries(ctx context.Context, orgID uuid.UUID) ([]modelSummary, error) {
	if h == nil || h.db == nil {
		return []modelSummary{}, nil
	}
	providerIDs, err := h.credentialBackedProviderIDs(ctx, orgID)
	if err != nil {
		return nil, err
	}

	reg := h.agentModelRegistry()
	canonical := reg.CanonicalModelsForProviders(providerIDs)
	out := make([]modelSummary, 0, len(canonical))
	for _, routed := range canonical {
		mdl := routed.Model
		if !registry.ModelSupportsTextOutput(mdl) {
			continue
		}
		out = append(out, modelSummary{
			ID:               mdl.ID,
			Name:             mdl.Name,
			ProviderIDs:      routed.ProviderIDs,
			Family:           mdl.Family,
			Reasoning:        mdl.Reasoning,
			ToolCall:         mdl.ToolCall,
			StructuredOutput: mdl.StructuredOutput,
			OpenWeights:      mdl.OpenWeights,
			Knowledge:        mdl.Knowledge,
			ReleaseDate:      mdl.ReleaseDate,
			NewFrom:          mdl.NewFrom,
			NewTo:            mdl.NewTo,
			Modalities:       mdl.Modalities,
			Cost:             mdl.Cost,
			Limit:            mdl.Limit,
			Status:           mdl.Status,
			Speed:            mdl.Speed,
			Description:      mdl.Description,
		})
	}
	return out, nil
}

func (h *AgentHandler) validateAgentSelectableModel(ctx context.Context, orgID uuid.UUID, modelID string) error {
	if modelID == "" {
		return fmt.Errorf("model is required")
	}
	if model, ok := h.agentModelRegistry().CanonicalModel(modelID); ok && !registry.ModelSupportsTextOutput(model.Model) {
		return fmt.Errorf("model %q does not support text output", modelID)
	}
	_, err := credentials.ResolveForModel(ctx, h.db, h.agentModelRegistry(), orgID, modelID)
	return err
}

func sandboxLogID(sb *model.Sandbox) string {
	if sb == nil {
		return ""
	}
	return sb.ID.String()
}

func (h *AgentHandler) credentialBackedProviderIDs(ctx context.Context, orgID uuid.UUID) ([]string, error) {
	var creds []model.Credential
	if err := h.db.WithContext(ctx).
		Where("revoked_at IS NULL AND (org_id = ? OR org_id IS NULL)", orgID).
		Order("created_at ASC").
		Find(&creds).Error; err != nil {
		return nil, fmt.Errorf("list active credentials: %w", err)
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(creds))
	for i := range creds {
		providerID := strings.TrimSpace(creds[i].ProviderID)
		if providerID == "" || seen[providerID] {
			continue
		}
		seen[providerID] = true
		out = append(out, providerID)
	}
	sort.Strings(out)
	return out, nil
}
