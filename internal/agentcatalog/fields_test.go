package agentcatalog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/usehivy/hivy/internal/model"
)

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

func TestValidateManifestsRejectsInvalidRuntimeTool(t *testing.T) {
	err := validateManifests([]Manifest{{
		Version: 1,
		Slug:    "hivy",
		Name:    "Hivy",
		Runtime: RuntimeManifest{SandboxImage: model.SandboxImageDefault},
		Tools:   map[string]any{"not_a_runtime_tool": true},
	}})
	if err == nil || !strings.Contains(err.Error(), "unknown runtime tool") {
		t.Fatalf("validateManifests error = %v, want unknown runtime tool", err)
	}
}

func TestGlobalHakareeManifestIncludesPortedSubAgents(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	manifests, err := loadManifests(filepath.Join(filepath.Dir(file), "..", "..", "global", "agents"))
	if err != nil {
		t.Fatalf("load global agents: %v", err)
	}
	if err := validateManifests(manifests); err != nil {
		t.Fatalf("validate global agents: %v", err)
	}

	var hakaree *Manifest
	for index := range manifests {
		if manifests[index].Slug == "hakaree-software-engineer" {
			hakaree = &manifests[index]
			break
		}
	}
	if hakaree == nil {
		t.Fatal("missing hakaree global agent manifest")
	}

	for _, key := range []string{"codebase-explorer", "librarian", "oracle"} {
		subAgent, ok := hakaree.SubAgents[key]
		if !ok {
			t.Fatalf("hakaree missing %q subagent; got keys=%v", key, sortedSubAgentKeys(hakaree.SubAgents))
		}
		if strings.TrimSpace(subAgent.Description) == "" {
			t.Fatalf("%s subagent has empty description", key)
		}
		if strings.TrimSpace(subAgent.instructions) == "" {
			t.Fatalf("%s subagent has empty loaded instructions", key)
		}
		assertManifestToolEnabled(t, subAgent.Tools, "read_file")
		assertManifestToolDisabled(t, subAgent.Tools, "write_file")
		assertManifestToolDisabled(t, subAgent.Tools, "apply_patch")
		assertManifestToolDisabled(t, subAgent.Tools, "subagent_task")
	}
	assertManifestToolEnabled(t, hakaree.SubAgents["codebase-explorer"].Tools, "multi_grep")
	assertManifestToolEnabled(t, hakaree.SubAgents["codebase-explorer"].Tools, "lsp")
	assertManifestToolEnabled(t, hakaree.SubAgents["librarian"].Tools, "bash")
	assertManifestToolEnabled(t, hakaree.SubAgents["librarian"].Tools, "check_bash_status")
	assertManifestToolEnabled(t, hakaree.SubAgents["oracle"].Tools, "multi_grep")
	assertManifestToolEnabled(t, hakaree.SubAgents["oracle"].Tools, "lsp")
	assertManifestToolDisabled(t, hakaree.SubAgents["oracle"].Tools, "bash")

	var stored map[string]model.AgentCatalogSubAgent
	if err := json.Unmarshal(catalogSubAgentsJSON(*hakaree), &stored); err != nil {
		t.Fatalf("decode stored subagents: %v", err)
	}
	for _, key := range []string{"codebase-explorer", "librarian", "oracle"} {
		if strings.TrimSpace(stored[key].Instructions) == "" {
			t.Fatalf("%s stored subagent has empty instructions", key)
		}
		assertManifestToolEnabled(t, stored[key].Tools, "read_file")
	}
}

func TestLoadManifestStoresRuntimeToolDefinitions(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "sub_agents", "codebase-explorer"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub_agents", "codebase-explorer", "instructions.md"), []byte("Trace code paths."), 0o600); err != nil {
		t.Fatalf("write instructions: %v", err)
	}
	manifestPath := filepath.Join(dir, "agent.json")
	manifestJSON := `{
  "version": 1,
  "slug": "hakaree",
  "name": "Hakaree",
  "runtime": {"sandbox_image": "developer", "model": "deepseek-v4-pro"},
  "tools": {
    "read_file": true,
    "grep": {"max_results": 25},
    "lsp": false
  },
  "mcp_tool_filter": {"deny": ["generate_image", "generate_vector_image", "generate_image", " "]},
  "prompt": {},
  "plugins": {},
  "sub_agents": {
    "codebase-explorer": {
      "name": "Codebase Explorer",
      "description": "Maps code paths.",
      "model": "qwen3.7-plus",
      "tools": {
        "read_file": true,
        "multi_grep": true
      },
      "mcp_tool_filter": {"allow": ["generate_image", "generate_vector_image", "generate_image", " "]},
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
	if manifest.McpToolFilter == nil {
		t.Fatal("manifest mcp tool filter missing")
	}
	updates := catalogUpdates(manifest, model.RawJSON("{}"), "hash", model.AgentCatalogStatusActive)
	tools, ok := updates["tools"].(model.JSON)
	if !ok {
		t.Fatalf("tools has type %T", updates["tools"])
	}
	if tools["read_file"] != true {
		t.Fatalf("read_file tool = %#v", tools["read_file"])
	}
	if _, ok := tools["lsp"]; ok {
		t.Fatalf("disabled lsp tool should be omitted: %#v", tools)
	}
	grep, ok := tools["grep"].(map[string]any)
	if !ok {
		t.Fatalf("grep config = %#v", tools["grep"])
	}
	if grep["max_results"] != float64(25) {
		t.Fatalf("grep max_results = %#v", grep["max_results"])
	}

	raw, ok := updates["sub_agents"].(model.RawJSON)
	if !ok {
		t.Fatalf("sub_agents has type %T", updates["sub_agents"])
	}
	var subAgents map[string]model.AgentCatalogSubAgent
	if err := json.Unmarshal(raw, &subAgents); err != nil {
		t.Fatalf("decode sub_agents: %v", err)
	}
	subTools := subAgents["codebase-explorer"].Tools
	if subTools["read_file"] != true || subTools["multi_grep"] != true {
		t.Fatalf("subagent tools = %#v", subTools)
	}
	subMCPFilter := subAgents["codebase-explorer"].McpToolFilter
	if subMCPFilter == nil || !reflect.DeepEqual(subMCPFilter.Allow, []string{"generate_image", "generate_vector_image"}) || subMCPFilter.Deny != nil {
		t.Fatalf("subagent mcp tool filter = %#v", subMCPFilter)
	}
}

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
	got := subAgents["codebase-explorer"]
	if got.Instructions != "Trace code paths with evidence." {
		t.Fatalf("instructions = %q", got.Instructions)
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
