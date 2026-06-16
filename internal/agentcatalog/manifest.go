package agentcatalog

import "encoding/json"

type Manifest struct {
	Version      int             `json:"version"`
	Slug         string          `json:"slug"`
	Name         string          `json:"name"`
	Description  string          `json:"description"`
	Category     string          `json:"category"`
	AvatarURL    string          `json:"avatar_url"`
	Developer    string          `json:"developer"`
	Official     *bool           `json:"official,omitempty"`
	Enabled      *bool           `json:"enabled,omitempty"`
	Default      *bool           `json:"default,omitempty"`
	Runtime      RuntimeManifest `json:"runtime"`
	Prompt       PromptManifest  `json:"prompt"`
	Plugins      PluginManifest  `json:"plugins"`
	raw          json.RawMessage `json:"-"`
	sourcePath   string          `json:"-"`
	dir          string          `json:"-"`
	instructions string          `json:"-"`
}

type RuntimeManifest struct {
	SandboxStrategy string `json:"sandbox_strategy"`
	Model           string `json:"model"`
	MultimodalModel string `json:"multimodal_model"`
}

type PromptManifest struct {
	Instructions string `json:"instructions"`
}

type PluginManifest struct {
	Required    []string `json:"required"`
	Recommended []string `json:"recommended"`
}
