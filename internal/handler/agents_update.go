package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

// Update handles PATCH /v1/agents/{id}.
// @Summary Update an agent
// @Description Updates a user-managed agent definition. The default Hivy agent cannot be renamed or moved away from always_on.
// @Tags agents
// @Accept json
// @Produce json
// @Param id path string true "Agent ID"
// @Param body body agentMutationRequest true "Agent update payload"
// @Success 200 {object} agentMutationResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/agents/{id} [patch]
func (h *AgentHandler) Update(w http.ResponseWriter, r *http.Request) {
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
	var req agentMutationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
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
		log.ErrorContext(ctx, "load agent for update", "error", err, "agent_id", agentID, "org_id", org.ID)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load agent"})
		return
	}

	updates := map[string]any{}
	if !h.applyAgentUpdateFields(w, ctx, &agent, &req, updates) {
		return
	}
	if len(updates) > 0 {
		if err := h.db.WithContext(ctx).
			Model(&model.Agent{}).
			Where("id = ? AND org_id = ?", agent.ID, org.ID).
			Updates(updates).Error; err != nil {
			if isDuplicateKeyError(err) {
				writeJSON(w, http.StatusConflict, errorResponse{Error: "agent name already exists"})
				return
			}
			log.ErrorContext(ctx, "update agent", "error", err, "agent_id", agent.ID, "org_id", org.ID)
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to update agent"})
			return
		}
	}
	writeJSON(w, http.StatusOK, agentMutationResponse{Agent: h.agentListItem(ctx, org.ID, agent)})
}

func (h *AgentHandler) applyAgentUpdateFields(w http.ResponseWriter, ctx context.Context, agent *model.Agent, req *agentMutationRequest, updates map[string]any) bool {
	if req.Name != nil {
		name := cleanStringPtr(req.Name)
		if name == "" {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "name cannot be empty"})
			return false
		}
		if agent.IsDefault && name != agent.Name {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "default agent cannot be renamed"})
			return false
		}
		updates["name"] = name
		agent.Name = name
	}
	if req.Description != nil {
		value := cleanStringPtr(req.Description)
		updates["description"] = value
		agent.Description = &value
	}
	if req.Instructions != nil {
		value := strings.TrimSpace(*req.Instructions)
		updates["instructions"] = value
		agent.Instructions = &value
	}
	if req.AvatarURL != nil {
		value := cleanStringPtr(req.AvatarURL)
		updates["avatar_url"] = optionalStringPtr(value)
		agent.AvatarURL = optionalStringPtr(value)
	}
	if req.Icon != nil {
		value := cleanStringPtr(req.Icon)
		updates["icon"] = value
		agent.Icon = value
	}
	if req.SandboxStrategy != nil {
		strategy := cleanStringPtr(req.SandboxStrategy)
		if !isValidAgentSandboxStrategy(strategy) {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "sandbox_strategy must be always_on or per_session"})
			return false
		}
		if agent.IsDefault && strategy != agentStrategyAlwaysOn {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "default agent must use always_on sandbox_strategy"})
			return false
		}
		updates["sandbox_strategy"] = strategy
		agent.SandboxStrategy = strategy
	}
	if req.SandboxTemplateID != nil {
		id, ok := parseOptionalUUIDForRequest(w, req.SandboxTemplateID, "sandbox_template_id")
		if !ok {
			return false
		}
		updates["sandbox_template_id"] = id
		agent.SandboxTemplateID = id
	}
	if req.Model != nil {
		modelID := cleanStringPtr(req.Model)
		credID, err := h.credentialIDForAgentModel(ctx, modelID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return false
		}
		updates["model"] = modelID
		updates["credential_id"] = credID
		agent.Model = modelID
		agent.CredentialID = credID
	}
	if req.Tools != nil {
		value := normalizeJSONPtr(req.Tools)
		updates["tools"] = value
		agent.Tools = value
	}
	if req.McpServers != nil {
		value, ok := normalizeMCPServersForRequest(w, req.McpServers)
		if !ok {
			return false
		}
		updates["mcp_servers"] = value
		agent.McpServers = value
	}
	if req.Skills != nil {
		value := normalizeJSONPtr(req.Skills)
		updates["skills"] = value
		agent.Skills = value
	}
	if req.Permissions != nil {
		value, ok := normalizePermissionsForRequest(w, req.Permissions)
		if !ok {
			return false
		}
		updates["permissions"] = value
		agent.Permissions = value
	}
	if req.Resources != nil {
		value := normalizeJSONPtr(req.Resources)
		updates["resources"] = value
		agent.Resources = value
	}
	if req.SandboxTools != nil {
		value, ok := normalizeSandboxToolsForRequest(w, req.SandboxTools)
		if !ok {
			return false
		}
		if agent.IsDefault && len(value) > 0 {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "default agent cannot enable workspace sandbox tools"})
			return false
		}
		updates["sandbox_tools"] = pq.StringArray(value)
		agent.SandboxTools = pq.StringArray(value)
	}
	return true
}
