package model

import (
	"encoding/json"
	"strings"
	"testing"
)

// nullEscape is the six-character sequence (backslash u 0 0 0 0) that Postgres
// jsonb rejects with SQLSTATE 22P05.
const nullEscape = "\\u0000"

// assertNoNullEscape mirrors Postgres jsonb: the NUL code point is not storable,
// so a Value() result must never contain its escaped form.
func assertNoNullEscape(t *testing.T, v any) {
	t.Helper()
	s, ok := v.(string)
	if !ok {
		t.Fatalf("Value() returned %T, want string", v)
	}
	if strings.Contains(s, nullEscape) {
		t.Fatalf("Value() output still contains the NUL escape: %q", s)
	}
}

func TestJSONValue_StripsNullBytes(t *testing.T) {
	j := JSON{
		"text":   "hello\x00world",
		"nested": map[string]any{"inner": "a\x00b"},
		"list":   []any{"x\x00y", "clean"},
		"num":    float64(42),
	}
	v, err := j.Value()
	if err != nil {
		t.Fatalf("Value() error: %v", err)
	}
	assertNoNullEscape(t, v)

	// Round-trips to valid JSON with the NUL removed but everything else intact.
	var got map[string]any
	if err := json.Unmarshal([]byte(v.(string)), &got); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}
	if got["text"] != "helloworld" {
		t.Errorf("text = %q, want %q", got["text"], "helloworld")
	}
	if got["num"] != float64(42) {
		t.Errorf("num = %v, want 42", got["num"])
	}
}

// A string whose literal text is the escape sequence (no real NUL rune) must be
// preserved, not corrupted by the sanitizer.
func TestJSONValue_PreservesLiteralEscapeText(t *testing.T) {
	j := JSON{"literal": nullEscape}
	v, err := j.Value()
	if err != nil {
		t.Fatalf("Value() error: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(v.(string)), &got); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}
	if got["literal"] != nullEscape {
		t.Errorf("literal = %q, want %q", got["literal"], nullEscape)
	}
}

func TestJSONValue_CommonPathUnchanged(t *testing.T) {
	j := JSON{"a": "b", "n": float64(1)}
	v, err := j.Value()
	if err != nil {
		t.Fatalf("Value() error: %v", err)
	}
	assertNoNullEscape(t, v)
}

func TestRawJSONValue_StripsNullBytes(t *testing.T) {
	raw, err := json.Marshal(map[string]any{"k": "a\x00b"})
	if err != nil {
		t.Fatalf("setup marshal: %v", err)
	}
	if !strings.Contains(string(raw), nullEscape) {
		t.Fatalf("setup: expected the NUL escape in %q", raw)
	}
	v, err := RawJSON(raw).Value()
	if err != nil {
		t.Fatalf("Value() error: %v", err)
	}
	assertNoNullEscape(t, v)
}
