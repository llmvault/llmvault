package agentcatalog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/usehivy/hivy/internal/model"
)

func TestLoadManifestLoadsSubAgentInstructions(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "sub_agents", "codebase-explorer"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	instructionsPath := filepath.Join(dir, "sub_agents", "codebase-explorer", "instructions.md")
	if err := os.WriteFile(instructionsPath, []byte("Trace code paths with evidence."), 0o600); err != nil {
		t.Fatalf("write instructions: %v", err)
	}
	manifestPath := filepath.Join(dir, "agent.json")
	manifestJSON := `{
  "version": 1,
  "slug": "hakaree",
  "name": "Hakaree",
  "runtime": {"model": "deepseek-v4-pro"},
  "prompt": {},
  "required_connections": [],
  "sub_agents": {
    "codebase-explorer": {
      "name": "Codebase Explorer",
      "description": "Maps code paths.",
      "model": "qwen3.7-plus",
      "prompt": {"instructions": "./sub_agents/codebase-explorer/instructions.md"}
    }
  }
}`
	if err := os.WriteFile(manifestPath, []byte(manifestJSON), 0o600); err != nil {
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
	if got := subAgents["codebase-explorer"].Instructions; got != "Trace code paths with evidence." {
		t.Fatalf("instructions = %q", got)
	}

	firstHash, err := sourceHash(manifest)
	if err != nil {
		t.Fatalf("first hash: %v", err)
	}
	if err := os.WriteFile(instructionsPath, []byte("Trace deeper paths."), 0o600); err != nil {
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
