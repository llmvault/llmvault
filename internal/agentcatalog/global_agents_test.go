package agentcatalog

import (
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestGlobalKaraManifestRequiresDesignPlugin(t *testing.T) {
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
		if manifests[index].Slug == "kara" {
			kara = &manifests[index]
			break
		}
	}
	if kara == nil {
		t.Fatal("missing kara global agent manifest")
	}
	if kara.Default == nil || !*kara.Default {
		t.Fatal("kara should be a default catalog agent")
	}
	if !reflect.DeepEqual(normalizeStrings(kara.Plugins.Required), []string{"design"}) {
		t.Fatalf("kara required plugins = %#v, want [design]", kara.Plugins.Required)
	}
	if strings.TrimSpace(kara.instructions) == "" {
		t.Fatal("kara instructions should be loaded")
	}
	assertManifestToolEnabled(t, kara.Tools, "bash")
	assertManifestToolEnabled(t, kara.Tools, "skill_view")
}
