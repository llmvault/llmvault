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
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/registry"
)

var agentAllowedModelIDs = []string{
	"deepseek-v4-flash",
	"step-3.7-flash",
	"ling-2.6-1t",
	"mimo-v2.5-pro",
}

type updateAgentModelRequest struct {
	Model string `json:"model"`
}

type updateAgentModelResponse struct {
	Agent agentResponse     `json:"agent"`
	Sync  syncAgentResponse `json:"sync"`
}

// ListModels handles GET /v1/agents/models.
// @Summary List agent-selectable models
// @Description Returns the OpenRouter-backed model allowlist supported for Hivy agents.
// @Tags agents
// @Produce json
// @Success 200 {array} modelSummary
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/agents/models [get]
func (h *AgentHandler) ListModels(w http.ResponseWriter, r *http.Request) {
	models, err := h.agentModelSummaries(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load agent models"})
		return
	}
	writeJSON(w, http.StatusOK, models)
}

// UpdateModel handles PATCH /v1/agents/{id}/model.
// @Summary Update an agent model
// @Description Persists Hivy's agent model and pushes the full runtime config to the live sandbox.
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
	if err := h.validateAgentSelectableModel(ctx, modelID); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
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
		log.ErrorContext(ctx, "load agent for model update", "error", err, "agent_id", agentID, "org_id", org.ID)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load agent"})
		return
	}

	if upgrade, ok, err := activeAgentSandboxUpgrade(ctx, h.db, org.ID, agentID); err != nil {
		log.ErrorContext(ctx, "load active agent sandbox upgrade for model update", "error", err, "agent_id", agentID)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load active upgrade"})
		return
	} else if ok {
		writeAgentUpgradeConflict(w, upgrade)
		return
	}

	cred, err := pickActiveSystemCredentialForModel(ctx, h.db, h.agentModelRegistry(), modelID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}

	if agent.Model != modelID || agent.CredentialID == nil || *agent.CredentialID != cred.ID {
		if err := h.db.WithContext(ctx).
			Model(&model.Agent{}).
			Where("id = ? AND org_id = ?", agent.ID, org.ID).
			Updates(map[string]any{
				"model":         modelID,
				"credential_id": cred.ID,
			}).Error; err != nil {
			log.ErrorContext(ctx, "update agent model", "error", err, "agent_id", agent.ID, "org_id", org.ID)
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to update agent model"})
			return
		}
		agent.Model = modelID
		agent.CredentialID = &cred.ID
	}

	sb, syncResp, err := h.SyncAgent(ctx, &agent)
	if err != nil {
		log.ErrorContext(ctx, "sync agent after model update", "error", err,
			"agent_id", agent.ID,
			"sandbox_id", sandboxLogID(sb),
			"model", modelID,
		)
		logging.Capture(ctx, fmt.Errorf("sync agent after model update: %w", err))
		writeJSON(w, http.StatusBadGateway, errorResponse{Error: "agent model saved, but sandbox rejected sync"})
		return
	}

	log.InfoContext(ctx, "agent model updated and synced",
		"agent_id", agent.ID,
		"sandbox_id", sandboxLogID(sb),
		"model", modelID,
	)
	writeJSON(w, http.StatusOK, updateAgentModelResponse{
		Agent: toAgentResponse(agent),
		Sync:  toSyncResponseDTO(syncResp),
	})
}

func (h *AgentHandler) agentModelRegistry() *registry.Registry {
	if h != nil && h.registry != nil {
		return h.registry
	}
	return registry.Global()
}

func (h *AgentHandler) agentModelSummaries(ctx context.Context) ([]modelSummary, error) {
	if h == nil || h.db == nil {
		return []modelSummary{}, nil
	}
	hasOpenRouter, err := hasActiveSystemCredentialForProvider(ctx, h.db, "openrouter")
	if err != nil {
		return nil, err
	}
	if !hasOpenRouter {
		return []modelSummary{}, nil
	}

	reg := h.agentModelRegistry()
	out := make([]modelSummary, 0, len(agentAllowedModelIDs))
	for _, id := range agentAllowedModelIDs {
		route, ok := reg.ResolveModel("openrouter", id)
		if !ok {
			continue
		}
		mdl := route.Model
		out = append(out, modelSummary{
			ID:               id,
			Name:             mdl.Name,
			ProviderIDs:      []string{"openrouter"},
			Family:           mdl.Family,
			Reasoning:        mdl.Reasoning,
			ToolCall:         mdl.ToolCall,
			StructuredOutput: mdl.StructuredOutput,
			OpenWeights:      mdl.OpenWeights,
			Knowledge:        mdl.Knowledge,
			ReleaseDate:      mdl.ReleaseDate,
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

func (h *AgentHandler) validateAgentSelectableModel(ctx context.Context, modelID string) error {
	if modelID == "" {
		return fmt.Errorf("model is required")
	}
	if !isAgentAllowedModelID(modelID) {
		return fmt.Errorf("model %q is not available for agents", modelID)
	}
	models, err := h.agentModelSummaries(ctx)
	if err != nil {
		return fmt.Errorf("load agent model allowlist: %w", err)
	}
	for _, mdl := range models {
		if mdl.ID == modelID {
			return nil
		}
	}
	return fmt.Errorf("model %q is not backed by an active OpenRouter system credential", modelID)
}

func isAgentAllowedModelID(modelID string) bool {
	for _, allowed := range agentAllowedModelIDs {
		if modelID == allowed {
			return true
		}
	}
	return false
}

func sandboxLogID(sb *model.Sandbox) string {
	if sb == nil {
		return ""
	}
	return sb.ID.String()
}

func hasActiveSystemCredentialForProvider(ctx context.Context, db *gorm.DB, providerID string) (bool, error) {
	var count int64
	if err := db.WithContext(ctx).
		Model(&model.Credential{}).
		Where("org_id IS NULL AND revoked_at IS NULL AND provider_id = ?", providerID).
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("count active system credentials: %w", err)
	}
	return count > 0, nil
}

func pickActiveSystemCredentialForModel(ctx context.Context, db *gorm.DB, reg *registry.Registry, modelID string) (*model.Credential, error) {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return nil, fmt.Errorf("model is required")
	}
	if reg == nil {
		reg = registry.Global()
	}

	if err := reg.ValidateCanonicalModel(modelID); err != nil {
		return nil, err
	}

	var creds []model.Credential
	if err := db.WithContext(ctx).
		Where("org_id IS NULL AND revoked_at IS NULL").
		Order("created_at ASC").
		Find(&creds).Error; err != nil {
		return nil, fmt.Errorf("list active system credentials: %w", err)
	}

	for i := range creds {
		if _, ok := reg.ResolveModel(creds[i].ProviderID, modelID); ok {
			return &creds[i], nil
		}
	}
	return nil, fmt.Errorf("model %q is not backed by an active system credential", modelID)
}
