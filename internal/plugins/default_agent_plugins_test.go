package plugins

import (
	"encoding/json"
	"testing"
)

// Runtime is universal across all agents. Builder plugins are installed only
// on each team's default Hivy agent; the remaining bundled capabilities stay
// team-managed.
func TestBundledPluginInstallDefaults(t *testing.T) {
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
		Locked              bool `json:"locked"`
	} {
		t.Helper()
		manifest, ok := bySlug[slug]
		if !ok {
			t.Fatalf("bundled plugin %q not found in global/plugins", slug)
		}
		var flags struct {
			DefaultAgentInstall bool `json:"default_agent_install"`
			AutoInstall         bool `json:"auto_install"`
			Locked              bool `json:"locked"`
		}
		if err := json.Unmarshal(manifest.raw, &flags); err != nil {
			t.Fatalf("decode %q manifest: %v", slug, err)
		}
		return flags
	}

	runtimeFlags := decode(t, "runtime")
	if !runtimeFlags.AutoInstall || !runtimeFlags.Locked {
		t.Fatalf("runtime must remain the locked auto-install plugin: %#v", runtimeFlags)
	}

	for _, slug := range []string{"agent-builder", "skill-manager"} {
		t.Run(slug, func(t *testing.T) {
			flags := decode(t, slug)
			if flags.AutoInstall || !flags.DefaultAgentInstall || flags.Locked {
				t.Fatalf("%q must install on default Hivy agents only: %#v", slug, flags)
			}
		})
	}

	for _, slug := range []string{"apps", "service-discovery", "sheets"} {
		t.Run(slug, func(t *testing.T) {
			flags := decode(t, slug)
			if flags.AutoInstall || flags.DefaultAgentInstall || flags.Locked {
				t.Fatalf("%q must be a team-managed optional plugin: %#v", slug, flags)
			}
		})
	}
}
