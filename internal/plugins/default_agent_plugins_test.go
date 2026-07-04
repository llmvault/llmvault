package plugins

import (
	"encoding/json"
	"testing"
)

// Every bundled plugin that must live on the org's default (Hivy) agent has to
// load through the same path boot sync uses and carry default_agent_install (and
// NOT auto_install, which would put it on every agent). If any of these flags is
// dropped, the plugin silently stops reaching the default agent — and, because
// PluginDetachLock keys off default_agent_install, also becomes user-removable
// from it. This guards all three at once.
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

	for _, slug := range []string{"agent-builder", "skill-manager", "service-discovery"} {
		t.Run(slug, func(t *testing.T) {
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
			if !flags.DefaultAgentInstall {
				t.Fatalf("%q must set default_agent_install: true", slug)
			}
			if flags.AutoInstall {
				t.Fatalf("%q must not be auto_install (default agent only)", slug)
			}
		})
	}
}
