package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

// Update handles PATCH /v1/agents/{id}.
// @Summary Update an agent
// @Description Updates a user-managed agent definition. The default Hivy agent cannot be renamed.
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
		Preload("AgentCatalog").
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
	var channelIDs []uuid.UUID
	if req.ChannelIDs != nil {
		var parsed bool
		channelIDs, parsed = h.normalizeAgentChannelIDs(ctx, w, org.ID, req.ChannelIDs)
		if !parsed {
			return
		}
	}
	if len(updates) > 0 || req.ChannelIDs != nil {
		if err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if len(updates) > 0 {
				if err := tx.Model(&model.Agent{}).
					Where("id = ? AND org_id = ?", agent.ID, org.ID).
					Updates(updates).Error; err != nil {
					return err
				}
			}
			if req.ChannelIDs != nil {
				if err := h.replaceAgentChannelsTx(tx, org.ID, agent.ID, channelIDs); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
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
	if req.SandboxImage != nil {
		image, ok := normalizeAgentSandboxImageForRequest(w, req.SandboxImage)
		if !ok {
			return false
		}
		updates["sandbox_image"] = image
		agent.SandboxImage = image
	}
	if req.SandboxSize != nil {
		size, ok := normalizeAgentSandboxSizeForRequest(w, req.SandboxSize)
		if !ok {
			return false
		}
		updates["sandbox_size"] = size
		agent.SandboxSize = size
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
		if err := h.validateAgentSelectableModel(ctx, *agent.OrgID, modelID); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return false
		}
		updates["model"] = modelID
		agent.Model = modelID
	}
	if req.AvailableModels != nil {
		value := normalizeAgentAvailableModels(agent.Model, req.AvailableModels)
		if err := h.validateAgentAvailableModels(ctx, *agent.OrgID, value); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return false
		}
		updates["available_models"] = value
		agent.AvailableModels = value
	}
	if !agentAllowsModel(agent, agent.Model) {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "model must be included in available_models"})
		return false
	}
	if req.ImageModel != nil {
		value := cleanStringPtr(req.ImageModel)
		if err := validateImageModelPreference(value, false); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return false
		}
		updates["image_model"] = value
		agent.ImageModel = value
	}
	if req.VectorImageModel != nil {
		value := cleanStringPtr(req.VectorImageModel)
		if err := validateImageModelPreference(value, true); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return false
		}
		updates["vector_image_model"] = value
		agent.VectorImageModel = value
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
