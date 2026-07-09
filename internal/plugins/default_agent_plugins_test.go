package plugins

import (
	"encoding/json"
	"testing"
)

// Bundled plugins that must reach every agent load through the same path boot
// sync uses and carry the auto_install flag. If the flag is dropped the plugin
// silently stops reaching its agents.
func TestBundledDefaultAgentPluginsCarryFlag(t *testing.T) {
	root, err := resolveDir("global/plugins")
	if err != nil {
		t.Fatalf("resolve global/plugins: %v", err)
	}
	manifests, err := loadManifests(root)
	if err != nil {
		t.Fatalf("load manifests: %v", err)
	}
	if err := validateManifests(manifests); err != nil {
		t.Fatalf("validate manifests: %v", err)
	}

	bySlug := map[string]*Manifest{}
	for i := range manifests {
		bySlug[manifests[i].Slug] = &manifests[i]
	}

	decode := func(t *testing.T, slug string) struct {
		DefaultAgentInstall bool `json:"default_agent_install"`
		AutoInstall         bool `json:"auto_install"`
	} {
		t.Helper()
		manifest, ok := bySlug[slug]
		if !ok {
			t.Fatalf("bundled plugin %q not found in global/plugins", slug)
		}
		var flags struct {
			DefaultAgentInstall bool `json:"default_agent_install"`
			AutoInstall         bool `json:"auto_install"`
		}
		if err := json.Unmarshal(manifest.raw, &flags); err != nil {
			t.Fatalf("decode %q manifest: %v", slug, err)
		}
		return flags
	}

	// System plugins: auto_install so they reach every team's agents.
	for _, slug := range []string{"agent-builder", "skill-manager", "service-discovery"} {
		t.Run(slug, func(t *testing.T) {
			flags := decode(t, slug)
			if !flags.AutoInstall {
				t.Fatalf("%q must set auto_install: true (system plugin)", slug)
			}
		})
	}
}
