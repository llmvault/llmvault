package agentcatalog

import (
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func TestGlobalAnnaManifestHasNoOptionalMCPGrants(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	manifests, err := loadManifests(filepath.Join(filepath.Dir(file), "..", "..", "global", "agents"))
	if err != nil {
		t.Fatalf("load global agents: %v", err)
	}

	var anna *Manifest
	for index := range manifests {
		if manifests[index].Slug == "anna-playwright-qa-engineer" {
			anna = &manifests[index]
			break
		}
	}
	if anna == nil {
		t.Fatal("missing Anna global agent manifest")
	}
	if anna.McpToolFilter == nil || !reflect.DeepEqual(anna.McpToolFilter.Allow, []string{}) {
		t.Fatalf("Anna mcp tool filter = %#v, want no optional grants", anna.McpToolFilter)
	}
}
