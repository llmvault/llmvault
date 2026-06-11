package employeeruntime

import (
	"testing"

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
