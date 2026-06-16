package handler

import (
	"reflect"
	"testing"

	"github.com/usehivy/hivy/internal/model"
)

func TestPluginPresentationMetadataDefaults(t *testing.T) {
	plugin := model.Plugin{
		Category:    "Developer Tools",
		Description: "Inspect your deployment workflows.",
		Developer:   "Hivy",
	}

	got := pluginPresentationMetadata(plugin)

	if got.DetailCategory != "Developer Tools" {
		t.Fatalf("DetailCategory = %q, want %q", got.DetailCategory, "Developer Tools")
	}
	if !got.Official {
		t.Fatalf("Official = false, want true")
	}
	if got.Featured {
		t.Fatalf("Featured = true, want false")
	}
	if !reflect.DeepEqual(got.Capabilities, []string{"Read"}) {
		t.Fatalf("Capabilities = %#v, want %#v", got.Capabilities, []string{"Read"})
	}
	if len(got.Examples) != 0 {
		t.Fatalf("Examples = %#v, want empty", got.Examples)
	}
	if got.Links != nil {
		t.Fatalf("Links = %#v, want nil", got.Links)
	}
	if got.LongDescription != "Inspect your deployment workflows." {
		t.Fatalf("LongDescription = %q, want %q", got.LongDescription, "Inspect your deployment workflows.")
	}
}

func TestPluginPresentationMetadataFromManifest(t *testing.T) {
	plugin := model.Plugin{
		Category:    "Developer Tools",
		Description: "Base description",
		Developer:   "Hivy",
		Manifest: model.RawJSON(`{
			"detail_category": "Productivity",
			"official": false,
			"featured": true,
			"capabilities": ["Read", "Write"],
			"examples": ["One", "Two"],
			"links": {
				"website": "https://example.com",
				"privacy": "https://example.com/privacy",
				"terms": "https://example.com/terms"
			},
			"long_description": "Detailed description"
		}`),
	}

	got := pluginPresentationMetadata(plugin)

	if got.DetailCategory != "Productivity" {
		t.Fatalf("DetailCategory = %q, want %q", got.DetailCategory, "Productivity")
	}
	if got.Official {
		t.Fatalf("Official = true, want false")
	}
	if !got.Featured {
		t.Fatalf("Featured = false, want true")
	}
	if !reflect.DeepEqual(got.Capabilities, []string{"Read", "Write"}) {
		t.Fatalf("Capabilities = %#v, want %#v", got.Capabilities, []string{"Read", "Write"})
	}
	if !reflect.DeepEqual(got.Examples, []string{"One", "Two"}) {
		t.Fatalf("Examples = %#v, want %#v", got.Examples, []string{"One", "Two"})
	}
	if got.Links == nil {
		t.Fatalf("Links = nil, want values")
	}
	if got.Links.Website != "https://example.com" || got.Links.Privacy != "https://example.com/privacy" || got.Links.Terms != "https://example.com/terms" {
		t.Fatalf("Links = %#v, want populated values", got.Links)
	}
	if got.LongDescription != "Detailed description" {
		t.Fatalf("LongDescription = %q, want %q", got.LongDescription, "Detailed description")
	}
}

func TestPluginPresentationMetadataInvalidManifestFallsBackToDefaults(t *testing.T) {
	plugin := model.Plugin{
		Category:    "Communication",
		Description: "Search messages.",
		Developer:   "Partner",
		Manifest:    model.RawJSON(`{"invalid"`),
	}

	got := pluginPresentationMetadata(plugin)

	if got.DetailCategory != "Communication" {
		t.Fatalf("DetailCategory = %q, want %q", got.DetailCategory, "Communication")
	}
	if got.Official {
		t.Fatalf("Official = true, want false")
	}
	if got.LongDescription != "Search messages." {
		t.Fatalf("LongDescription = %q, want %q", got.LongDescription, "Search messages.")
	}
}
