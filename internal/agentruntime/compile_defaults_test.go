package agentruntime

import (
	"testing"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/model"
)

// jsonArray must decode a JSON array stored in a RawJSON column; the old
// map[string]any code could never represent an array and returned empty.
func TestJsonArray_MCPServersArrayRoundtrip(t *testing.T) {
	raw := model.RawJSON(`[{"name":"hivy","transport":{"type":"streamable_http","url":"https://mcp.example.com"}}]`)
	got := jsonArray(raw)
	if len(got) != 1 {
		t.Fatalf("jsonArray: got len %d, want 1; raw=%s", len(got), string(raw))
	}
	entry, ok := got[0].(map[string]any)
	if !ok {
		t.Fatalf("jsonArray: element is %T, want map[string]any", got[0])
	}
	if entry["name"] != "hivy" {
		t.Fatalf("jsonArray: name = %q, want hivy", entry["name"])
	}
}

// An empty JSON array must produce an empty slice (not nil, not a panic).
func TestJsonArray_EmptyArray(t *testing.T) {
	got := jsonArray(model.RawJSON("[]"))
	if got == nil || len(got) != 0 {
		t.Fatalf("jsonArray([]): got %v, want []", got)
	}
}

// Backward-compat: rows with '{}' stored must return an empty slice, not panic.
func TestJsonArray_EmptyObject(t *testing.T) {
	got := jsonArray(model.RawJSON("{}"))
	if len(got) != 0 {
		t.Fatalf("jsonArray({}): got len %d, want 0", len(got))
	}
}

// A nil/zero-length RawJSON returns an empty slice.
func TestJsonArray_Nil(t *testing.T) {
	got := jsonArray(model.RawJSON(nil))
	if len(got) != 0 {
		t.Fatalf("jsonArray(nil): got len %d, want 0", len(got))
	}
}

func TestProxyModelUsesHivyOpenRouterAppName(t *testing.T) {
	got := ProxyModelConfig(&config.Config{}, "deepseek-v4-flash", "")

	if got.ExtraHeaders["HTTP-Referer"] != "https://usehivy.com" {
		t.Fatalf("HTTP-Referer = %q, want https://usehivy.com", got.ExtraHeaders["HTTP-Referer"])
	}
	if got.ExtraHeaders["X-Title"] != "Hivy" {
		t.Fatalf("X-Title = %q, want Hivy", got.ExtraHeaders["X-Title"])
	}
}

func TestDefaultLimitsUseQuadrupledAgentRunBudgets(t *testing.T) {
	limits := defaultLimits()
	want := map[string]any{
		"max_turns_per_session":     200,
		"input_token_budget":        720000,
		"output_token_budget":       32000,
		"tool_call_timeout_seconds": 240,
	}
	for key, expected := range want {
		if limits[key] != expected {
			t.Fatalf("%s = %#v, want %#v", key, limits[key], expected)
		}
	}
}
