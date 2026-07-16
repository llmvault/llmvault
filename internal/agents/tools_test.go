package agents

import (
	"strings"
	"testing"

	"github.com/usehivy/hivy/internal/model"
)

func TestSplitTools_RoutesRuntimeAndMCP(t *testing.T) {
	runtime, mcpAllow, err := SplitTools([]string{"bash", "read_file", "web_search", "generate_image"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if runtime["bash"] != true || runtime["read_file"] != true {
		t.Fatalf("runtime tools = %#v, want bash+read_file", runtime)
	}
	if _, ok := runtime["web_search"]; ok {
		t.Fatalf("web_search must not land in runtime map: %#v", runtime)
	}
	if len(mcpAllow) != 2 || mcpAllow[0] != "generate_image" || mcpAllow[1] != "web_search" {
		t.Fatalf("mcpAllow = %#v, want sorted [generate_image web_search]", mcpAllow)
	}
}

func TestSplitTools_UnknownToolReturnsHelpfulError(t *testing.T) {
	_, _, err := SplitTools([]string{"bash", "totally_made_up"})
	if err == nil {
		t.Fatal("expected error for unknown tool")
	}
	msg := err.Error()
	if !strings.Contains(msg, `unknown tool "totally_made_up"`) {
		t.Fatalf("error should name the offending tool: %q", msg)
	}
	// Must enumerate the full allowed set for the model to self-correct.
	for _, want := range []string{"bash", "read_file", "web_search", "web_crawl", "generate_image", "sheet_list", "app_create"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error should list allowed tool %q; got %q", want, msg)
		}
	}
}

func TestSplitTools_DedupesMCP(t *testing.T) {
	_, mcpAllow, err := SplitTools([]string{"web_search", "web_search", " web_search "})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mcpAllow) != 1 || mcpAllow[0] != "web_search" {
		t.Fatalf("mcpAllow = %#v, want single web_search", mcpAllow)
	}
}

func TestAssignableToolIDs_IsUnionOfRuntimeAndMCP(t *testing.T) {
	ids := AssignableToolIDs()
	set := map[string]bool{}
	for _, id := range ids {
		set[id] = true
	}
	for _, id := range model.RuntimeBuiltInToolIDs {
		if !set[id] {
			t.Fatalf("enum missing runtime tool %q", id)
		}
	}
	for _, id := range AssignableMCPTools {
		if !set[id] {
			t.Fatalf("enum missing MCP tool %q", id)
		}
	}
	// Privileged tools are never assignable.
	for _, forbidden := range []string{toolCreateAgent, toolUpdateAgent, toolListTeamSkills} {
		if set[forbidden] {
			t.Fatalf("privileged tool %q must not be assignable", forbidden)
		}
	}
	if want := len(model.RuntimeBuiltInToolIDs) + len(AssignableMCPTools); len(ids) != want {
		t.Fatalf("enum length = %d, want %d", len(ids), want)
	}
}

func TestAgentBuilderEnabled(t *testing.T) {
	cases := []struct {
		name  string
		agent *model.Agent
		want  bool
	}{
		{"default agent enabled", &model.Agent{IsDefault: true}, true},
		{"nil filter not default -> disabled", &model.Agent{IsDefault: false}, false},
		{
			"explicit allow create_agent -> enabled",
			&model.Agent{McpToolFilter: &model.ToolFilter{Allow: []string{"web_search", "create_agent"}}},
			true,
		},
		{
			"explicit allow update_agent -> enabled",
			&model.Agent{McpToolFilter: &model.ToolFilter{Allow: []string{"update_agent"}}},
			true,
		},
		{
			"allow list without builder tools -> disabled",
			&model.Agent{McpToolFilter: &model.ToolFilter{Allow: []string{"web_search"}}},
			false,
		},
		{"nil agent -> disabled", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := agentBuilderEnabled(tc.agent); got != tc.want {
				t.Fatalf("agentBuilderEnabled = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSkillsJSON_MatchesResolverShape(t *testing.T) {
	got := skillsJSON([]string{"beta", "alpha"})
	// The resolver reads skills["skill_filter"]["allow"].
	allow := skillFilterAllow(got)
	if len(allow) != 2 {
		t.Fatalf("round-trip allow = %#v, want 2 entries", allow)
	}
	// Empty slugs -> empty object (nil filter = all allowed).
	if empty := skillsJSON(nil); len(empty) != 0 {
		t.Fatalf("empty skills = %#v, want {}", empty)
	}
}
