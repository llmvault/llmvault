package agentcatalog

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/usehivy/hivy/internal/model"
)

func TestGlobalKaraManifestRoutesImageGenerationThroughImageGenerator(t *testing.T) {
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

	var kara *Manifest
	for index := range manifests {
		if manifests[index].Slug == "kara-ui-and-graphics-designer" {
			kara = &manifests[index]
			break
		}
	}
	if kara == nil {
		t.Fatal("missing kara global agent manifest")
	}

	imageTools := []string{"generate_image", "generate_vector_image", "remix_image"}
	if kara.McpToolFilter == nil || !reflect.DeepEqual(kara.McpToolFilter.Deny, imageTools) {
		t.Fatalf("kara mcp tool filter = %#v", kara.McpToolFilter)
	}

	imageGenerator, ok := kara.SubAgents["image-generator"]
	if !ok {
		t.Fatalf("kara missing image-generator subagent; got keys=%v", sortedSubAgentKeys(kara.SubAgents))
	}
	if strings.TrimSpace(imageGenerator.instructions) == "" {
		t.Fatal("image-generator subagent has empty loaded instructions")
	}
	if !reflect.DeepEqual(imageGenerator.McpToolFilter.Allow, imageTools) {
		t.Fatalf("image-generator mcp tool filter = %#v", imageGenerator.McpToolFilter)
	}
	assertManifestToolDisabled(t, imageGenerator.Tools, "bash")
	assertManifestToolDisabled(t, imageGenerator.Tools, "read_file")

	designWorker := kara.SubAgents["design-worker"]
	if designWorker.McpToolFilter == nil || !reflect.DeepEqual(designWorker.McpToolFilter.Deny, imageTools) {
		t.Fatalf("design-worker mcp tool filter = %#v", designWorker.McpToolFilter)
	}

	var stored map[string]model.AgentCatalogSubAgent
	if err := json.Unmarshal(catalogSubAgentsJSON(*kara), &stored); err != nil {
		t.Fatalf("decode stored subagents: %v", err)
	}
	storedImageGenerator := stored["image-generator"]
	if storedImageGenerator.McpToolFilter == nil || !reflect.DeepEqual(storedImageGenerator.McpToolFilter.Allow, imageTools) {
		t.Fatalf("stored image-generator mcp tool filter = %#v", storedImageGenerator.McpToolFilter)
	}
}
