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
	ConnectionID string              `json:"connection_id,omitempty"`
	TriggerKeys  []string            `json:"trigger_keys,omitempty"`
	Conditions   *model.TriggerMatch `json:"conditions,omitempty"`
	Instructions string              `json:"instructions,omitempty"`
	// SecretKey is the optional plaintext shared secret for HTTP triggers.
	// When provided, the server bcrypt-hashes it before storing. Never returned
	// in any response — see agentTriggerResponse.SecretSet for visibility.
	SecretKey string `json:"secret_key,omitempty"`
}

type agentTriggerResponse struct {
	ID           string   `json:"id"`
	TriggerType  string   `json:"trigger_type"`
	ConnectionID string   `json:"connection_id,omitempty"`
	Provider     string   `json:"provider,omitempty"`
	TriggerKeys  []string `json:"trigger_keys,omitempty"`
	Enabled      bool     `json:"enabled"`
	Conditions   any      `json:"conditions,omitempty"`
	Instructions string   `json:"instructions,omitempty"`
	// SecretSet indicates whether an HTTP trigger has a shared secret configured.
	// True when the trigger requires auth on incoming requests. The secret value
	// is never returned.
	SecretSet bool `json:"secret_set"`
}

type agentSkillSummary struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	SourceType  string  `json:"source_type"`
	Locked      bool    `json:"locked,omitempty"`
	Required    bool    `json:"required,omitempty"`
}

type agentResponse struct {
	ID                    string                 `json:"id"`
	Name                  string                 `json:"name"`
	Description           *string                `json:"description,omitempty"`
	Instructions          string                 `json:"instructions"`
	AvatarURL             *string                `json:"avatar_url,omitempty"`
	Icon                  string                 `json:"icon"`
	Placeholder           string                 `json:"placeholder"`
	IsDefault             bool                   `json:"is_default"`
	SandboxStrategy       string                 `json:"sandbox_strategy"`
	SandboxTemplateID     *string                `json:"sandbox_template_id,omitempty"`
	Model                 string                 `json:"model"`
	Tools                 model.JSON             `json:"tools"`
	McpServers            json.RawMessage        `json:"mcp_servers"`
	Skills                model.JSON             `json:"skills"`
	Permissions           model.JSON             `json:"permissions"`
	SandboxTools          []string               `json:"sandbox_tools"`
	Status                string                 `json:"status"`
	LastMemoryRefreshedAt *string                `json:"last_memory_refreshed_at,omitempty"`
	MemoryRefreshStatus   string                 `json:"memory_refresh_status,omitempty"`
	MemoryRefreshError    string                 `json:"memory_refresh_error,omitempty"`
	Resources             model.JSON             `json:"resources"`
	Triggers              []agentTriggerResponse `json:"triggers"`
	AttachedSkills        []agentSkillSummary    `json:"attached_skills"`
	CreatedAt             string                 `json:"created_at"`
	UpdatedAt             string                 `json:"updated_at"`
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
	strategy := a.SandboxStrategy
	if strategy == "" {
		strategy = agentStrategyAlwaysOn
	}
	mcpServers := json.RawMessage(a.McpServers)
	if len(mcpServers) == 0 {
		mcpServers = json.RawMessage("[]")
	}
	resp := agentResponse{
		ID:                  a.ID.String(),
		Name:                fallbackAgentName(a),
		Description:         &description,
		Instructions:        instructions,
		AvatarURL:           &avatarURL,
		Icon:                a.Icon,
		Placeholder:         a.Placeholder,
		IsDefault:           a.IsDefault,
		SandboxStrategy:     strategy,
		Model:               a.Model,
		Tools:               nonNilJSON(a.Tools),
		McpServers:          mcpServers,
		Skills:              nonNilJSON(a.Skills),
		Permissions:         nonNilJSON(a.Permissions),
		SandboxTools:        append([]string(nil), a.SandboxTools...),
		Status:              a.Status,
		Resources:           nonNilJSON(a.Resources),
		MemoryRefreshStatus: a.MemoryRefreshStatus,
		MemoryRefreshError:  a.MemoryRefreshError,
		CreatedAt:           a.CreatedAt.Format(time.RFC3339),
		UpdatedAt:           a.UpdatedAt.Format(time.RFC3339),
	}
	if a.LastMemoryRefreshedAt != nil {
		s := a.LastMemoryRefreshedAt.Format(time.RFC3339)
		resp.LastMemoryRefreshedAt = &s
	}
	if a.SandboxTemplateID != nil {
		s := a.SandboxTemplateID.String()
		resp.SandboxTemplateID = &s
	}
	return resp
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
