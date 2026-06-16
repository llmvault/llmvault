package plugins

import "encoding/json"

type Manifest struct {
	Version             int                     `json:"version"`
	Slug                string                  `json:"slug"`
	Name                string                  `json:"name"`
	Description         string                  `json:"description"`
	Category            string                  `json:"category"`
	DetailCategory      string                  `json:"detail_category,omitempty"`
	Icon                string                  `json:"icon"`
	IconColor           string                  `json:"icon_color"`
	Developer           string                  `json:"developer"`
	Official            *bool                   `json:"official,omitempty"`
	Featured            *bool                   `json:"featured,omitempty"`
	Capabilities        []string                `json:"capabilities,omitempty"`
	Examples            []string                `json:"examples,omitempty"`
	Links               *ManifestLinks          `json:"links,omitempty"`
	LongDescription     string                  `json:"long_description,omitempty"`
	PluginVersion       string                  `json:"plugin_version"`
	Enabled             *bool                   `json:"enabled,omitempty"`
	RequiredConnections []ConnectionRequirement `json:"required_connections,omitempty"`
	raw                 json.RawMessage         `json:"-"`
	sourcePath          string                  `json:"-"`
	dir                 string                  `json:"-"`
}

type ManifestLinks struct {
	Website string `json:"website,omitempty"`
	Privacy string `json:"privacy,omitempty"`
	Terms   string `json:"terms,omitempty"`
}

type ConnectionRequirement struct {
	Provider string `json:"provider"`
	Kind     string `json:"kind,omitempty"`
	Required *bool  `json:"required,omitempty"`
}

type skillManifest struct {
	Name                         string              `json:"name"`
	Description                  string              `json:"description"`
	Category                     string              `json:"category"`
	Root                         string              `json:"root"`
	Tags                         []string            `json:"tags"`
	RequiredEnvironmentVariables []string            `json:"required_environment_variables"`
	Files                        []skillManifestFile `json:"files"`
}

type skillManifestFile struct {
	Path string `json:"path"`
	URL  string `json:"url"`
}

type loadedSkill struct {
	Manifest skillManifest
	Slug     string
	Dir      string
	Content  string
	Files    map[string]string
}
