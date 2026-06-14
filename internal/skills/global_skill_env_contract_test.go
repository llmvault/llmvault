package skills_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/usehivy/hivy/internal/agentruntime"
)

func TestBundledProviderProxySkillsDeclareRuntimeEnv(t *testing.T) {
	dir, err := repoRootForTest()
	if err != nil {
		t.Fatal(err)
	}

	for _, spec := range agentruntime.ServiceProxyEnvSpecs() {
		pluginManifest, err := os.ReadFile(filepath.Join(dir, "global/plugins", spec.SkillName, "plugin.json"))
		if err != nil {
			t.Fatalf("read bundled %s plugin manifest: %v", spec.SkillName, err)
		}
		var pluginParsed struct {
			RequiredConnections []struct {
				Provider string `json:"provider"`
			} `json:"required_connections"`
		}
		if err := json.Unmarshal(pluginManifest, &pluginParsed); err != nil {
			t.Fatalf("decode %s plugin manifest: %v", spec.SkillName, err)
		}
		if !containsPluginConnection(pluginParsed.RequiredConnections, spec.Provider) {
			t.Fatalf("%s required_connections = %#v, want %q", spec.SkillName, pluginParsed.RequiredConnections, spec.Provider)
		}

		manifest, err := os.ReadFile(filepath.Join(dir, "global/plugins", spec.SkillName, "skills", spec.SkillName, "skill.json"))
		if err != nil {
			t.Fatalf("read bundled %s manifest: %v", spec.SkillName, err)
		}
		var parsed struct {
			RequiredEnvironmentVariables []string `json:"required_environment_variables"`
		}
		if err := json.Unmarshal(manifest, &parsed); err != nil {
			t.Fatalf("decode %s manifest: %v", spec.SkillName, err)
		}
		if !containsString(parsed.RequiredEnvironmentVariables, spec.BaseURLEnv) ||
			!containsString(parsed.RequiredEnvironmentVariables, spec.AuthEnv) {
			t.Fatalf("%s required env vars = %#v, want %q and %q",
				spec.SkillName, parsed.RequiredEnvironmentVariables, spec.BaseURLEnv, spec.AuthEnv)
		}
	}
}

func repoRootForTest() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			if _, err := os.Stat(filepath.Join(dir, "global/plugins")); err == nil {
				return dir, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func containsPluginConnection(values []struct {
	Provider string `json:"provider"`
}, want string) bool {
	for _, value := range values {
		if value.Provider == want {
			return true
		}
	}
	return false
}
