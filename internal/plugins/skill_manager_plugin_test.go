package plugins

import (
	"context"
	"encoding/json"
	"testing"
)

// The bundled skill-manager plugin must load through the same path boot sync
// uses: default_agent_install so it lands on every org's default Hivy agent,
// one skill-creator skill, and all three reference files present.
func TestBundledSkillManagerPluginLoads(t *testing.T) {
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

	var manifest *Manifest
	for i := range manifests {
		if manifests[i].Slug == "skill-manager" {
			manifest = &manifests[i]
			break
		}
	}
	if manifest == nil {
		t.Fatal("skill-manager plugin not found in global/plugins")
	}
	var flags struct {
		DefaultAgentInstall bool `json:"default_agent_install"`
		AutoInstall         bool `json:"auto_install"`
	}
	if err := json.Unmarshal(manifest.raw, &flags); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if !flags.DefaultAgentInstall {
		t.Fatal("skill-manager must set default_agent_install: true")
	}
	if flags.AutoInstall {
		t.Fatal("skill-manager must not be auto_install (default agent only)")
	}

	skills, err := loadSkills(context.Background(), *manifest)
	if err != nil {
		t.Fatalf("load skill-manager skills: %v", err)
	}
	if len(skills) != 1 || skills[0].Slug != "skill-creator" {
		t.Fatalf("expected one skill-creator skill, got %#v", skills)
	}
	skill := skills[0]
	if skill.Content == "" || skill.Manifest.Description == "" {
		t.Fatal("skill-creator content and description must be non-empty")
	}
	for _, path := range []string{
		"references/security-review.md",
		"references/hivy-skill-format.md",
		"references/sandbox-environment.md",
		"references/compatibility-checklist.md",
	} {
		if skill.Files[path] == "" {
			t.Fatalf("skill-creator missing bundled file %s", path)
		}
	}
}
