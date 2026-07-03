package agentcatalog

import (
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
