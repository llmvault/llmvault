package model

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strings"
)

// RawJSON stores arbitrary JSON values (objects, arrays, strings, etc.) in JSONB columns.
// Unlike JSON (which is map[string]any), RawJSON can represent any valid JSON value.
type RawJSON json.RawMessage

func (r RawJSON) Value() (driver.Value, error) {
	if len(r) == 0 {
		return "null", nil
	}
	return sanitizeJSONBValue([]byte(r))
}

func (r *RawJSON) Scan(value any) error {
	if value == nil {
		*r = RawJSON("null")
		return nil
	}
	switch v := value.(type) {
	case string:
		*r = RawJSON(v)
	case []byte:
		*r = RawJSON(v)
	default:
		return fmt.Errorf("unsupported type for RawJSON: %T", value)
	}
	return nil
}

func (r RawJSON) MarshalJSON() ([]byte, error) {
	if len(r) == 0 {
		return []byte("null"), nil
	}
	return []byte(r), nil
}

func (r *RawJSON) UnmarshalJSON(data []byte) error {
	*r = RawJSON(data)
	return nil
}

// JSON is a custom type for JSONB columns in PostgreSQL.
type JSON map[string]any

func (j JSON) Value() (driver.Value, error) {
	if j == nil {
		return "{}", nil
	}
	b, err := json.Marshal(j)
	if err != nil {
		return nil, fmt.Errorf("marshaling JSON: %w", err)
	}
	return sanitizeJSONBValue(b)
}

func (j *JSON) Scan(value any) error {
	if value == nil {
		*j = JSON{}
		return nil
	}

	var raw []byte
	switch v := value.(type) {
	case string:
		raw = []byte(v)
	case []byte:
		raw = v
	default:
		return fmt.Errorf("unsupported type for JSON: %T", value)
	}

	return json.Unmarshal(raw, j)
}

// jsonbNullEscape is the JSON-encoded NUL code point (the six ASCII bytes
// backslash-u-0-0-0-0). Postgres jsonb rejects it with SQLSTATE 22P05
// ("unsupported Unicode escape sequence").
var jsonbNullEscape = []byte("\\u0000")

// sanitizeJSONBValue returns marshaled JSON bytes as a driver value safe for a
// Postgres jsonb column. Payloads occasionally carry NUL (U+0000) bytes (e.g.
// raw tool output streamed by the runtime); jsonb cannot store them, so when the
// escape is present we decode, strip the NUL runes, and re-encode. The common
// (NUL-free) path is a single bytes.Contains check with no extra allocation, and
// undecodable input falls back to the original bytes so no new failure mode is
// introduced.
func sanitizeJSONBValue(b []byte) (driver.Value, error) {
	if !bytes.Contains(b, jsonbNullEscape) {
		return string(b), nil
	}
	var decoded any
	if err := json.Unmarshal(b, &decoded); err != nil {
		return string(b), nil
	}
	clean, err := json.Marshal(stripNullBytes(decoded))
	if err != nil {
		return string(b), nil
	}
	return string(clean), nil
}

// stripNullBytes returns a copy of v with NUL (U+0000) runes removed from every
// string key and value. Non-string scalars are returned unchanged.
func stripNullBytes(v any) any {
	switch t := v.(type) {
	case string:
		if !strings.ContainsRune(t, 0) {
			return t
		}
		return strings.ReplaceAll(t, "\x00", "")
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			if strings.ContainsRune(k, 0) {
				k = strings.ReplaceAll(k, "\x00", "")
			}
			out[k] = stripNullBytes(val)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = stripNullBytes(val)
		}
		return out
	default:
		return v
	}
}
