package handler

import (
	"encoding/json"
	"net/http"

	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	pluginstore "github.com/usehivy/hivy/internal/plugins"
)

type agentMutationRequest struct {
	Name              *string          `json:"name,omitempty"`
	Description       *string          `json:"description,omitempty"`
	Instructions      *string          `json:"instructions,omitempty"`
	AvatarURL         *string          `json:"avatar_url,omitempty"`
	Icon              *string          `json:"icon,omitempty"`
	SandboxStrategy   *string          `json:"sandbox_strategy,omitempty"`
	SandboxImage      *string          `json:"sandbox_image,omitempty"`
	SandboxSize       *string          `json:"sandbox_size,omitempty"`
	SandboxTemplateID *string          `json:"sandbox_template_id,omitempty"`
	Model             *string          `json:"model,omitempty"`
	AvailableModels   *[]string        `json:"available_models,omitempty"`
	ImageModel        *string          `json:"image_model,omitempty"`
	VectorImageModel  *string          `json:"vector_image_model,omitempty"`
	Tools             *model.JSON      `json:"tools,omitempty"`
	McpServers        *json.RawMessage `json:"mcp_servers,omitempty"`
	Skills            *model.JSON      `json:"skills,omitempty"`
	Permissions       *model.JSON      `json:"permissions,omitempty"`
	Resources         *model.JSON      `json:"resources,omitempty"`
	SandboxTools      *[]string        `json:"sandbox_tools,omitempty"`
	ChannelIDs        *[]string        `json:"channel_ids,omitempty"`
}

type agentMutationResponse struct {
	Agent agentListItem `json:"agent"`
}

// Create handles POST /v1/agents.
// @Summary Create an agent
// @Description Creates a user-managed agent definition. Sandbox creation stays lazy until the first session.
// @Tags agents
// @Accept json
// @Produce json
// @Param body body agentMutationRequest true "Agent create payload"
// @Success 201 {object} agentMutationResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/agents [post]
func (h *AgentHandler) Create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logging.FromContext(ctx)
	org, ok := middleware.OrgFromContext(ctx)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing org context"})
		return
	}

	var req agentMutationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}

	name := cleanStringPtr(req.Name)
	if name == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "name is required"})
		return
	}
	modelID := cleanStringPtr(req.Model)
	if modelID == "" {
		modelID = agentruntime.DefaultAgentModel
	}
	availableModels := normalizeAgentAvailableModels(modelID, req.AvailableModels)
	if err := h.validateAgentAvailableModels(ctx, org.ID, availableModels); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	if !containsString(availableModels, modelID) {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "model must be included in available_models"})
		return
	}
	if err := h.validateAgentSelectableModel(ctx, org.ID, modelID); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	imageModel := cleanStringPtr(req.ImageModel)
	if err := validateImageModelPreference(imageModel, false); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	vectorImageModel := cleanStringPtr(req.VectorImageModel)
	if err := validateImageModelPreference(vectorImageModel, true); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}

	strategy := cleanStringPtr(req.SandboxStrategy)
	if strategy == "" {
		strategy = agentStrategyPerSession
	}
	if !isValidAgentSandboxStrategy(strategy) {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "sandbox_strategy must be always_on or per_session"})
		return
	}
	sandboxImage, ok := normalizeAgentSandboxImageForRequest(w, req.SandboxImage)
	if !ok {
		return
	}
	sandboxSize, ok := normalizeAgentSandboxSizeForRequest(w, req.SandboxSize)
	if !ok {
		return
	}
	sandboxTemplateID, ok := parseOptionalUUIDForRequest(w, req.SandboxTemplateID, "sandbox_template_id")
	if !ok {
		return
	}
	mcpServers, ok := normalizeMCPServersForRequest(w, req.McpServers)
	if !ok {
		return
	}
	// Every agent is created with the full toolset enabled. Tool selection is
	// not exposed at creation time, so requested permissions and sandbox tools
	// are ignored in favour of the canonical "all enabled" defaults.
	permissions := model.JSON{}
	for _, id := range model.BuiltInToolIDs() {
		permissions[id] = true
	}
	sandboxTools := make([]string, 0, len(model.ValidSandboxTools))
	for _, tool := range model.ValidSandboxTools {
		sandboxTools = append(sandboxTools, tool.ID)
	}

	desc := cleanStringPtr(req.Description)
	instructions := cleanStringPtr(req.Instructions)
	avatarURL := cleanStringPtr(req.AvatarURL)
	agent := model.Agent{
		OrgID:             &org.ID,
		Name:              name,
		Description:       &desc,
		Instructions:      &instructions,
		AvatarURL:         optionalStringPtr(avatarURL),
		Icon:              cleanStringPtr(req.Icon),
		IsDefault:         false,
		SandboxStrategy:   strategy,
		SandboxImage:      sandboxImage,
		SandboxSize:       sandboxSize,
		SandboxTemplateID: sandboxTemplateID,
		Model:             modelID,
		AvailableModels:   availableModels,
		ImageModel:        imageModel,
		VectorImageModel:  vectorImageModel,
		Tools:             normalizeJSONPtr(req.Tools),
		McpServers:        mcpServers,
		Skills:            normalizeJSONPtr(req.Skills),
		Permissions:       permissions,
		Resources:         normalizeJSONPtr(req.Resources),
		SandboxTools:      pq.StringArray(sandboxTools),
		Status:            "active",
	}
	channelIDs, ok := h.normalizeAgentChannelIDs(ctx, w, org.ID, req.ChannelIDs)
	if !ok {
		return
	}

	if err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&agent).Error; err != nil {
			return err
		}
		if err := h.replaceAgentChannelsTx(tx, org.ID, agent.ID, channelIDs); err != nil {
			return err
		}
		return pluginstore.EnsureAutoInstalledForAgent(ctx, tx, org.ID, agent.ID)
	}); err != nil {
		if isDuplicateKeyError(err) {
			writeJSON(w, http.StatusConflict, errorResponse{Error: "agent name already exists"})
			return
		}
		log.ErrorContext(ctx, "create agent", "error", err, "org_id", org.ID)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to create agent"})
		return
	}
	enqueueCanvasOrgSync(ctx, h.enqueuer, org.ID, "agent_create")
	writeJSON(w, http.StatusCreated, agentMutationResponse{Agent: h.agentListItem(ctx, org.ID, agent)})
}
