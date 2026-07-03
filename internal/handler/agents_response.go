package handler

import (
	"encoding/json"
	"time"

	"github.com/usehivy/hivy/internal/model"
)

// agentTriggerInput defines an agent trigger.
// TriggerType may be "webhook" (default) or "http".
type agentTriggerInput struct {
	ID           string              `json:"id,omitempty"`
	TriggerType  string              `json:"trigger_type,omitempty"` // "webhook" (default), "http"
	ChannelID    string              `json:"channel_id,omitempty"`
	ConnectionID string              `json:"connection_id,omitempty"`
	TriggerKeys  []string            `json:"trigger_keys,omitempty"`
	Conditions   *model.TriggerMatch `json:"conditions,omitempty"`
	SourceSlug   string              `json:"source_slug,omitempty"`
	Instructions string              `json:"instructions,omitempty"`
	// SecretKey is the optional plaintext shared secret for HTTP triggers.
	// When provided, the server bcrypt-hashes it before storing. Never returned
	// in any response — see agentTriggerResponse.SecretSet for visibility.
	SecretKey string `json:"secret_key,omitempty"`
}

type agentTriggerResponse struct {
	ID           string   `json:"id"`
	TriggerType  string   `json:"trigger_type"`
	ChannelID    string   `json:"channel_id,omitempty"`
	ConnectionID string   `json:"connection_id,omitempty"`
	Provider     string   `json:"provider,omitempty"`
	TriggerKeys  []string `json:"trigger_keys,omitempty"`
	TriggerKey   string   `json:"trigger_key,omitempty"`
	TriggerValue string   `json:"trigger_value,omitempty"`
	Enabled      bool     `json:"enabled"`
	Conditions   any      `json:"conditions,omitempty"`
	SourceSlug   string   `json:"source_slug,omitempty"`
	Instructions string   `json:"instructions,omitempty"`
	// SecretSet indicates whether an HTTP trigger has a shared secret configured.
	// True when the trigger requires auth on incoming requests. The secret value
	// is never returned.
	SecretSet bool `json:"secret_set"`
}

type agentSkillSummary struct {
	ID               string  `json:"id"`
	Name             string  `json:"name"`
	Description      *string `json:"description,omitempty"`
	HumanDescription *string `json:"human_description,omitempty"`
	SourceType       string  `json:"source_type"`
	Locked           bool    `json:"locked,omitempty"`
	Required         bool    `json:"required,omitempty"`
}

type agentCatalogSummary struct {
	ID                 string   `json:"id"`
	Slug               string   `json:"slug"`
	Name               string   `json:"name"`
	Description        string   `json:"description"`
	Category           string   `json:"category"`
	AvatarURL          string   `json:"avatar_url"`
	Developer          string   `json:"developer"`
	Official           bool     `json:"official"`
	IsDefault          bool     `json:"is_default"`
	SandboxImage       string   `json:"sandbox_image"`
	RequiredPlugins    []string `json:"required_plugins"`
	RecommendedPlugins []string `json:"recommended_plugins"`
}

type agentResponse struct {
	ID                string                 `json:"id"`
	Name              string                 `json:"name"`
	Description       *string                `json:"description,omitempty"`
	Instructions      string                 `json:"instructions"`
	AvatarURL         *string                `json:"avatar_url,omitempty"`
	Icon              string                 `json:"icon"`
	IsDefault         bool                   `json:"is_default"`
	SandboxImage      string                 `json:"sandbox_image"`
	SandboxSize       string                 `json:"sandbox_size"`
	SandboxTemplateID *string                `json:"sandbox_template_id,omitempty"`
	Model             string                 `json:"model"`
	ImageModel        string                 `json:"image_model"`
	VectorImageModel  string                 `json:"vector_image_model"`
	Tools             model.JSON             `json:"tools"`
	McpToolFilter     *model.ToolFilter      `json:"mcp_tool_filter,omitempty"`
	McpServers        json.RawMessage        `json:"mcp_servers"`
	Skills            model.JSON             `json:"skills"`
	Permissions       model.JSON             `json:"permissions"`
	SandboxTools      []string               `json:"sandbox_tools"`
	ChannelIDs        []string               `json:"channel_ids"`
	Status            string                 `json:"status"`
	Catalog           *agentCatalogSummary   `json:"catalog,omitempty"`
	Resources         model.JSON             `json:"resources"`
	Triggers          []agentTriggerResponse `json:"triggers"`
	AttachedSkills    []agentSkillSummary    `json:"attached_skills"`
	SubAgents         []subAgentResponse     `json:"sub_agents"`
	CreatedAt         string                 `json:"created_at"`
	UpdatedAt         string                 `json:"updated_at"`
}

func toAgentResponse(a model.Agent) agentResponse {
	description := hivyAgentDescription
	if a.Description != nil && *a.Description != "" {
		description = *a.Description
	}
	avatarURL := hivyAgentAvatarURL
	if a.AvatarURL != nil && *a.AvatarURL != "" {
		avatarURL = *a.AvatarURL
	}
	instructions := ""
	if a.Instructions != nil {
		instructions = *a.Instructions
	}
	sandboxSize := model.NormalizeTemplateSize(a.SandboxSize)
	mcpServers := json.RawMessage(a.McpServers)
	if len(mcpServers) == 0 {
		mcpServers = json.RawMessage("[]")
	}
	resp := agentResponse{
		ID:               a.ID.String(),
		Name:             fallbackAgentName(a),
		Description:      &description,
		Instructions:     instructions,
		AvatarURL:        &avatarURL,
		Icon:             a.Icon,
		IsDefault:        a.IsDefault,
		SandboxImage:     model.NormalizeSandboxImage(a.SandboxImage),
		SandboxSize:      sandboxSize,
		Model:            a.Model,
		ImageModel:       a.ImageModel,
		VectorImageModel: a.VectorImageModel,
		Tools:            nonNilJSON(a.Tools),
		McpToolFilter:    a.McpToolFilter,
		McpServers:       mcpServers,
		Skills:           nonNilJSON(a.Skills),
		Permissions:      nonNilJSON(a.Permissions),
		SandboxTools:     append([]string(nil), a.SandboxTools...),
		Status:           a.Status,
		Resources:        nonNilJSON(a.Resources),
		SubAgents:        []subAgentResponse{},
		CreatedAt:        a.CreatedAt.Format(time.RFC3339),
		UpdatedAt:        a.UpdatedAt.Format(time.RFC3339),
	}
	if a.SandboxTemplateID != nil {
		s := a.SandboxTemplateID.String()
		resp.SandboxTemplateID = &s
	}
	if a.AgentCatalog != nil {
		resp.Catalog = toAgentCatalogSummary(*a.AgentCatalog)
	}
	return resp
}

func toAgentCatalogSummary(c model.AgentCatalog) *agentCatalogSummary {
	return &agentCatalogSummary{
		ID:                 c.ID.String(),
		Slug:               c.Slug,
		Name:               c.Name,
		Description:        c.Description,
		Category:           c.Category,
		AvatarURL:          c.AvatarURL,
		Developer:          c.Developer,
		Official:           c.Official,
		IsDefault:          c.IsDefault,
		SandboxImage:       model.NormalizeSandboxImage(c.SandboxImage),
		RequiredPlugins:    append([]string(nil), c.RequiredPlugins...),
		RecommendedPlugins: append([]string(nil), c.RecommendedPlugins...),
	}
}

func fallbackAgentName(a model.Agent) string {
	if a.Name != "" {
		return a.Name
	}
	return hivyAgentName
}

func nonNilJSON(value model.JSON) model.JSON {
	if value == nil {
		return model.JSON{}
	}
	return value
}
