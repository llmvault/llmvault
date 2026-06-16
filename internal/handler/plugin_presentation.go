package handler

import (
	"encoding/json"
	"strings"

	"github.com/usehivy/hivy/internal/model"
	pluginmeta "github.com/usehivy/hivy/internal/plugins"
)

func pluginPresentationMetadata(plugin model.Plugin) pluginPresentation {
	developer := strings.TrimSpace(plugin.Developer)
	if developer == "" {
		developer = "Hivy"
	}
	out := pluginPresentation{
		DetailCategory:  plugin.Category,
		Official:        strings.EqualFold(developer, "Hivy"),
		Featured:        false,
		Capabilities:    []string{"Read"},
		Examples:        []string{},
		LongDescription: plugin.Description,
	}

	if len(plugin.Manifest) == 0 {
		return out
	}

	var manifest pluginmeta.Manifest
	if err := json.Unmarshal(plugin.Manifest, &manifest); err != nil {
		return out
	}

	if value := strings.TrimSpace(manifest.DetailCategory); value != "" {
		out.DetailCategory = value
	}
	if manifest.Official != nil {
		out.Official = *manifest.Official
	}
	if manifest.Featured != nil {
		out.Featured = *manifest.Featured
	}
	if capabilities := normalizePluginStrings(manifest.Capabilities); len(capabilities) > 0 {
		out.Capabilities = capabilities
	}
	if examples := normalizePluginStrings(manifest.Examples); len(examples) > 0 {
		out.Examples = examples
	}
	if manifest.Links != nil {
		links := &pluginLinksResponse{
			Website: strings.TrimSpace(manifest.Links.Website),
			Privacy: strings.TrimSpace(manifest.Links.Privacy),
			Terms:   strings.TrimSpace(manifest.Links.Terms),
		}
		if links.Website != "" || links.Privacy != "" || links.Terms != "" {
			out.Links = links
		}
	}
	if value := strings.TrimSpace(manifest.LongDescription); value != "" {
		out.LongDescription = value
	}
	return out
}

func normalizePluginStrings(items []string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
