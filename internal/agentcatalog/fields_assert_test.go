package agentcatalog

import (
	"sort"
	"testing"
)

func assertManifestToolEnabled(t *testing.T, tools map[string]any, key string) {
	t.Helper()
	if !toolSelectionEnabled(tools[key]) {
		t.Fatalf("tool %q is not enabled in %#v", key, tools)
	}
}

func assertManifestToolDisabled(t *testing.T, tools map[string]any, key string) {
	t.Helper()
	if toolSelectionEnabled(tools[key]) {
		t.Fatalf("tool %q should not be enabled in %#v", key, tools)
	}
}

func sortedSubAgentKeys(subAgents map[string]SubAgentManifest) []string {
	keys := make([]string, 0, len(subAgents))
	for key := range subAgents {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
