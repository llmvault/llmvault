package agentcatalog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/lib/pq"

	"github.com/usehivy/hivy/internal/model"
)

func TestCatalogUpdatesDefaultsAvailableModelsToRuntimeModel(t *testing.T) {
	updates := catalogUpdates(Manifest{
		Runtime: RuntimeManifest{Model: "deepseek-v4-flash"},
	}, model.RawJSON("{}"), "hash", model.AgentCatalogStatusActive)

	got, ok := updates["available_models"].(pq.StringArray)
	if !ok {
		t.Fatalf("available_models has type %T", updates["available_models"])
	}
	want := []string{"deepseek-v4-flash"}
	if !reflect.DeepEqual([]string(got), want) {
		t.Fatalf("available_models = %#v, want %#v", got, want)
	}
}

func TestCatalogUpdatesNormalizesSandboxImage(t *testing.T) {
	updates := catalogUpdates(Manifest{
		Runtime: RuntimeManifest{SandboxImage: " developer "},
	}, model.RawJSON("{}"), "hash", model.AgentCatalogStatusActive)

	if got, want := updates["sandbox_image"], model.SandboxImageDeveloper; got != want {
		t.Fatalf("sandbox_image = %q, want %q", got, want)
	}
}

func TestCatalogUpdatesDefaultsSandboxImage(t *testing.T) {
	updates := catalogUpdates(Manifest{}, model.RawJSON("{}"), "hash", model.AgentCatalogStatusActive)

	if got, want := updates["sandbox_image"], model.SandboxImageDefault; got != want {
		t.Fatalf("sandbox_image = %q, want %q", got, want)
	}
}

func TestValidateManifestsRejectsInvalidSandboxImage(t *testing.T) {
	err := validateManifests([]Manifest{{
		Version: 1,
		Slug:    "hakaree",
		Name:    "Hakaree",
		Runtime: RuntimeManifest{SandboxImage: "builder"},
	}})
	if err == nil {
		t.Fatal("validateManifests succeeded, want invalid sandbox_image error")
	}
}

func TestLoadManifestLoadsSubAgentInstructions(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "sub_agents", "codebase-explorer"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	instructionsPath := filepath.Join(dir, "sub_agents", "codebase-explorer", "instructions.md")
	if err := os.WriteFile(instructionsPath, []byte("Trace code paths with evidence."), 0o644); err != nil {
		t.Fatalf("write instructions: %v", err)
	}
	manifestPath := filepath.Join(dir, "agent.json")
	manifestJSON := `{
  "version": 1,
  "slug": "hakaree",
  "name": "Hakaree",
  "runtime": {"model": "deepseek-v4-pro"},
  "prompt": {},
  "plugins": {},
  "sub_agents": {
    "codebase-explorer": {
      "name": "Codebase Explorer",
      "description": "Maps code paths.",
      "model": "qwen3.7-plus",
      "prompt": {"instructions": "./sub_agents/codebase-explorer/instructions.md"}
    }
  }
}`
	if err := os.WriteFile(manifestPath, []byte(manifestJSON), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	manifest, err := loadManifest(manifestPath)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}
	if err := validateManifests([]Manifest{manifest}); err != nil {
		t.Fatalf("validate manifest: %v", err)
	}
	updates := catalogUpdates(manifest, model.RawJSON("{}"), "hash", model.AgentCatalogStatusActive)
	raw, ok := updates["sub_agents"].(model.RawJSON)
	if !ok {
		t.Fatalf("sub_agents has type %T", updates["sub_agents"])
	}
	var subAgents map[string]model.AgentCatalogSubAgent
	if err := json.Unmarshal(raw, &subAgents); err != nil {
		t.Fatalf("decode sub_agents: %v", err)
	}
	got := subAgents["codebase-explorer"]
	if got.Instructions != "Trace code paths with evidence." {
		t.Fatalf("instructions = %q", got.Instructions)
	}

	firstHash, err := sourceHash(manifest)
	if err != nil {
		t.Fatalf("first hash: %v", err)
	}
	if err := os.WriteFile(instructionsPath, []byte("Trace deeper paths."), 0o644); err != nil {
		t.Fatalf("rewrite instructions: %v", err)
	}
	updatedManifest, err := loadManifest(manifestPath)
	if err != nil {
		t.Fatalf("reload manifest: %v", err)
	}
	secondHash, err := sourceHash(updatedManifest)
	if err != nil {
		t.Fatalf("second hash: %v", err)
	}
	if firstHash == secondHash {
		t.Fatal("source hash should change when subagent instructions change")
	}
}

func TestCatalogUpdatesNormalizesAvailableModels(t *testing.T) {
	updates := catalogUpdates(Manifest{
		Runtime: RuntimeManifest{
			Model: "deepseek-v4-flash",
			AvailableModels: []string{
				" gemini-3-flash-preview ",
				"",
				"deepseek-v4-flash",
				"gemini-3-flash-preview",
			},
		},
	}, model.RawJSON("{}"), "hash", model.AgentCatalogStatusActive)

	got, ok := updates["available_models"].(pq.StringArray)
	if !ok {
		t.Fatalf("available_models has type %T", updates["available_models"])
	}
	want := []string{"gemini-3-flash-preview", "deepseek-v4-flash"}
	if !reflect.DeepEqual([]string(got), want) {
		t.Fatalf("available_models = %#v, want %#v", got, want)
	}
}
