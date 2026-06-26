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
