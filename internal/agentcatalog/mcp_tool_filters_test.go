package agentcatalog

import (
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"testing"
)

// Every shipped agent must state its optional native MCP surface explicitly.
// Skill loading is universal and injected by the compiler, so it is purposely
// absent from these manifest-level lists.
func TestGlobalAgentManifestsUseLeastPrivilegeMCPAllowLists(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	manifests, err := loadManifests(filepath.Join(filepath.Dir(file), "..", "..", "global", "agents"))
	if err != nil {
		t.Fatalf("load global agents: %v", err)
	}

	want := map[string][]string{
		"anna-playwright-qa-engineer": {},
		"hakaree-software-engineer":   {},
		"hivy": {
			"search_knowledge_base", "manage_memories",
			"list_team_plugins", "list_agents", "get_agent", "create_agent", "update_agent",
			"create_team_plugin", "create_skill", "update_skill", "archive_skill",
			"cron", "create_http_trigger", "list_channels",
		},
		"kara-ui-and-graphics-designer": {},
		"pedro-lead-gen": {
			"sheet_create", "sheet_list", "sheet_describe", "sheet_manage",
			"rows_query", "rows_write", "sheet_import_csv", "sheet_operations",
		},
		"ricky-app-builder": {
			"app_create", "app_publish", "app_status", "app_logs", "app_rollback",
			"sheet_create", "sheet_list", "sheet_describe", "sheet_manage",
			"rows_query", "rows_write", "sheet_import_csv", "sheet_operations",
		},
		"zuko-code-reviewer": {},
	}

	if len(manifests) != len(want) {
		t.Fatalf("global manifest count = %d, want %d", len(manifests), len(want))
	}
	for _, manifest := range manifests {
		expected, ok := want[manifest.Slug]
		if !ok {
			t.Fatalf("unexpected global agent %q", manifest.Slug)
		}
		if manifest.McpToolFilter == nil {
			t.Fatalf("%s has no explicit mcp_tool_filter", manifest.Slug)
		}
		if len(manifest.McpToolFilter.Deny) != 0 {
			t.Fatalf("%s uses deny rules instead of explicit allows: %#v", manifest.Slug, manifest.McpToolFilter)
		}
		got := append([]string(nil), manifest.McpToolFilter.Allow...)
		expected = append([]string(nil), expected...)
		sort.Strings(got)
		sort.Strings(expected)
		if !reflect.DeepEqual(got, expected) {
			t.Fatalf("%s mcp allow = %v, want %v", manifest.Slug, got, expected)
		}
	}
}
