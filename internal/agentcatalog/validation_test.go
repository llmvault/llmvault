package agentcatalog

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/usehivy/hivy/internal/model"
)

func TestValidateManifestsRejectsDefaultAgentWithoutRequiredPlugins(t *testing.T) {
	isDefault := true
	err := validateManifests([]Manifest{{
		Version: 1,
		Slug:    "builder",
		Name:    "Builder",
		Default: &isDefault,
		Runtime: RuntimeManifest{SandboxImage: model.SandboxImageDefault},
	}})
	if err == nil || !strings.Contains(err.Error(), "must declare at least one required plugin") {
		t.Fatalf("validateManifests error = %v, want missing required plugin error", err)
	}
}

func TestValidateManifestsRejectsInvalidReasoningEffort(t *testing.T) {
	err := validateManifests([]Manifest{{
		Version: 1,
		Slug:    "hakaree",
		Name:    "Hakaree",
		Runtime: RuntimeManifest{SandboxImage: model.SandboxImageDefault, ReasoningEffort: "extreme"},
	}})
	if err == nil || !strings.Contains(err.Error(), "invalid runtime reasoning_effort") {
		t.Fatalf("validateManifests error = %v, want invalid reasoning_effort error", err)
	}
}

func TestValidateManifestsAcceptsReasoningEffort(t *testing.T) {
	err := validateManifests([]Manifest{{
		Version: 1,
		Slug:    "hakaree",
		Name:    "Hakaree",
		Runtime: RuntimeManifest{SandboxImage: model.SandboxImageDefault, ReasoningEffort: "Medium"},
	}})
	if err != nil {
		t.Fatalf("validateManifests error = %v, want nil for valid reasoning_effort", err)
	}
}

func TestCatalogUpdatesNormalizesReasoningEffort(t *testing.T) {
	updates := catalogUpdates(Manifest{
		Runtime: RuntimeManifest{ReasoningEffort: " High "},
	}, model.RawJSON("{}"), "hash", model.AgentCatalogStatusActive)

	if got, want := updates["default_reasoning_effort"], "high"; got != want {
		t.Fatalf("default_reasoning_effort = %q, want %q", got, want)
	}

	var row model.AgentCatalog
	applyCatalogUpdates(&row, updates)
	if row.DefaultReasoningEffort != "high" {
		t.Fatalf("row.DefaultReasoningEffort = %q, want high", row.DefaultReasoningEffort)
	}
}

func TestValidateManifestsAcceptsAutoLoadSkills(t *testing.T) {
	err := validateManifests([]Manifest{{
		Version: 1,
		Slug:    "hakaree",
		Name:    "Hakaree",
		Runtime: RuntimeManifest{SandboxImage: model.SandboxImageDefault},
		AutoLoadSkills: model.AutoLoadSkills{
			{Name: "qa-registry"},
			{Name: "browser", Files: []string{"references/commands.md"}},
		},
	}})
	if err != nil {
		t.Fatalf("validateManifests error = %v, want nil for valid auto_load_skills", err)
	}
}

func TestValidateManifestsRejectsAutoLoadSkillTraversalPath(t *testing.T) {
	err := validateManifests([]Manifest{{
		Version: 1,
		Slug:    "hakaree",
		Name:    "Hakaree",
		Runtime: RuntimeManifest{SandboxImage: model.SandboxImageDefault},
		AutoLoadSkills: model.AutoLoadSkills{
			{Name: "browser", Files: []string{"../etc/passwd"}},
		},
	}})
	if err == nil || !strings.Contains(err.Error(), "auto_load_skills") {
		t.Fatalf("validateManifests error = %v, want auto_load_skills path error", err)
	}
}

func TestValidateManifestsRejectsSubAgentAutoLoadSkillEmptyName(t *testing.T) {
	err := validateManifests([]Manifest{{
		Version: 1,
		Slug:    "hakaree",
		Name:    "Hakaree",
		Runtime: RuntimeManifest{SandboxImage: model.SandboxImageDefault},
		SubAgents: map[string]SubAgentManifest{
			"executor": {
				Name:           "Executor",
				AutoLoadSkills: model.AutoLoadSkills{{Name: "  "}},
			},
		},
	}})
	if err == nil || !strings.Contains(err.Error(), "auto_load_skills") {
		t.Fatalf("validateManifests error = %v, want subagent auto_load_skills error", err)
	}
}

func TestCatalogUpdatesNormalizesAutoLoadSkills(t *testing.T) {
	updates := catalogUpdates(Manifest{
		AutoLoadSkills: model.AutoLoadSkills{
			{Name: " qa-registry "},
			{Name: "browser", Files: []string{"references/commands.md"}},
		},
	}, model.RawJSON("{}"), "hash", model.AgentCatalogStatusActive)

	got := updates["auto_load_skills"].(model.AutoLoadSkills)
	if len(got) != 2 || got[0].Name != "qa-registry" {
		t.Fatalf("auto_load_skills = %#v", got)
	}

	var row model.AgentCatalog
	applyCatalogUpdates(&row, updates)
	if len(row.AutoLoadSkills) != 2 || row.AutoLoadSkills[1].Files[0] != "references/commands.md" {
		t.Fatalf("row.AutoLoadSkills = %#v", row.AutoLoadSkills)
	}
}

func TestCatalogSubAgentsJSONIncludesAutoLoadSkills(t *testing.T) {
	raw := catalogSubAgentsJSON(Manifest{
		SubAgents: map[string]SubAgentManifest{
			"executor": {
				Name:           "Executor",
				AutoLoadSkills: model.AutoLoadSkills{{Name: "browser", Files: []string{"references/commands.md"}}},
			},
		},
	})
	var decoded map[string]model.AgentCatalogSubAgent
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		t.Fatalf("unmarshal sub_agents json: %v", err)
	}
	spec := decoded["executor"]
	if len(spec.AutoLoadSkills) != 1 || spec.AutoLoadSkills[0].Name != "browser" || spec.AutoLoadSkills[0].Files[0] != "references/commands.md" {
		t.Fatalf("subagent AutoLoadSkills = %#v", spec.AutoLoadSkills)
	}
}

func TestValidateManifestsRejectsHivyPluginDeclarations(t *testing.T) {
	isDefault := true
	err := validateManifests([]Manifest{{
		Version: 1,
		Slug:    "hivy",
		Name:    "Hivy",
		Default: &isDefault,
		Runtime: RuntimeManifest{SandboxImage: model.SandboxImageDefault},
		Plugins: PluginManifest{Required: []string{"runtime"}},
	}})
	if err == nil || !strings.Contains(err.Error(), "default Hivy agent must not declare plugins") {
		t.Fatalf("validateManifests error = %v, want Hivy plugin declaration error", err)
	}
}
