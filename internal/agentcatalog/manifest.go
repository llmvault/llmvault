package agentcatalog

import "encoding/json"

type Manifest struct {
	Version       int                         `json:"version"`
	Slug          string                      `json:"slug"`
	Name          string                      `json:"name"`
	Description   string                      `json:"description"`
	Category      string                      `json:"category"`
	AvatarURL     string                      `json:"avatar_url"`
	Developer     string                      `json:"developer"`
	AgentVersion  string                      `json:"agent_version"`
	Official      *bool                       `json:"official,omitempty"`
	Enabled       *bool                       `json:"enabled,omitempty"`
	Default       *bool                       `json:"default,omitempty"`
	Runtime       RuntimeManifest             `json:"runtime"`
	Tools         map[string]any              `json:"tools,omitempty"`
	McpToolFilter *ToolFilterManifest         `json:"mcp_tool_filter,omitempty"`
	SkillFilter   *SkillFilterManifest        `json:"skill_filter,omitempty"`
	Prompt        PromptManifest              `json:"prompt"`
	Plugins       PluginManifest              `json:"plugins"`
	SubAgents     map[string]SubAgentManifest `json:"sub_agents,omitempty"`
	raw           json.RawMessage             `json:"-"`
	sourcePath    string                      `json:"-"`
	dir           string                      `json:"-"`
	instructions  string                      `json:"-"`
}

type RuntimeManifest struct {
	SandboxImage    string   `json:"sandbox_image"`
	Model           string   `json:"model"`
	AvailableModels []string `json:"available_models"`
}

type PromptManifest struct {
	Instructions string `json:"instructions"`
}

type PluginManifest struct {
	Required    []string `json:"required"`
	Recommended []string `json:"recommended"`
}

type SkillFilterManifest struct {
	Allow []string `json:"allow,omitempty"`
}

type ToolFilterManifest struct {
	Allow []string `json:"allow,omitempty"`
	Deny  []string `json:"deny,omitempty"`
}

type SubAgentManifest struct {
	Name          string               `json:"name"`
	Description   string               `json:"description"`
	Model         string               `json:"model"`
	Tools         map[string]any       `json:"tools,omitempty"`
	McpToolFilter *ToolFilterManifest  `json:"mcp_tool_filter,omitempty"`
	SkillFilter   *SkillFilterManifest `json:"skill_filter,omitempty"`
	Prompt        PromptManifest       `json:"prompt"`
	instructions  string               `json:"-"`
}
