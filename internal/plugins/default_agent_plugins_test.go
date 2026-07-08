package plugins

import (
	"encoding/json"
	"testing"
)

// Bundled plugins that must reach the org's default (Hivy) agent have to load
// through the same path boot sync uses and carry the right install flag. Two
// classes:
//   - default-agent-only (agent-builder): default_agent_install, NOT auto_install
//     — it belongs on the default agent, not on every agent.
//   - system/auto-install (skill-manager, service-discovery): auto_install, which
//     puts them on every agent of every team (a superset of the default agent).
//
// If a flag is dropped the plugin silently stops reaching its agents. This
// guards both classes at once.
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

	// Default-agent-only: default_agent_install, never auto_install.
	for _, slug := range []string{"agent-builder"} {
		t.Run(slug, func(t *testing.T) {
			flags := decode(t, slug)
			if !flags.DefaultAgentInstall {
				t.Fatalf("%q must set default_agent_install: true", slug)
			}
			if flags.AutoInstall {
				t.Fatalf("%q must not be auto_install (default agent only)", slug)
			}
		})
	}

	// System plugins: auto_install so they reach every team's agents.
	for _, slug := range []string{"skill-manager", "service-discovery"} {
		t.Run(slug, func(t *testing.T) {
			flags := decode(t, slug)
			if !flags.AutoInstall {
				t.Fatalf("%q must set auto_install: true (system plugin)", slug)
			}
		})
	}
}
