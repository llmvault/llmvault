package skills_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/usehivy/hivy/internal/agentruntime"
)

func TestBundledProviderProxySkillsDeclareRuntimeEnv(t *testing.T) {
	dir, err := skillsForRepoTest()
	if err != nil {
		t.Fatal(err)
	}

	for _, spec := range agentruntime.ServiceProxyEnvSpecs() {
		manifest, err := os.ReadFile(filepath.Join(dir, "global/skills", spec.SkillName, "skill.json"))
		if err != nil {
			t.Fatalf("read bundled %s manifest: %v", spec.SkillName, err)
		}
		var parsed struct {
			IntegrationIDs               []string `json:"integration_ids"`
			RequiredEnvironmentVariables []string `json:"required_environment_variables"`
		}
		if err := json.Unmarshal(manifest, &parsed); err != nil {
			t.Fatalf("decode %s manifest: %v", spec.SkillName, err)
		}
		if !containsString(parsed.IntegrationIDs, spec.Provider) {
			t.Fatalf("%s integration_ids = %#v, want %q", spec.SkillName, parsed.IntegrationIDs, spec.Provider)
		}
		if !containsString(parsed.RequiredEnvironmentVariables, spec.BaseURLEnv) ||
			!containsString(parsed.RequiredEnvironmentVariables, spec.AuthEnv) {
			t.Fatalf("%s required env vars = %#v, want %q and %q",
				spec.SkillName, parsed.RequiredEnvironmentVariables, spec.BaseURLEnv, spec.AuthEnv)
		}
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
