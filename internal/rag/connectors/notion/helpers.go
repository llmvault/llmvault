package notion

import (
	"sort"
	"strings"
	"time"
)

// getString returns the string at key, or "" if absent or not a string.
func getString(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	s, _ := m[key].(string)
	return s
}

// getBool returns the bool at key, or false if absent or not a bool.
func getBool(m map[string]any, key string) bool {
	if m == nil {
		return false
	}
	b, _ := m[key].(bool)
	return b
}

// blockTypeObj returns the block's type-specific payload (the object
// stored under the block's own "type" key), or nil if absent.
func blockTypeObj(m map[string]any) map[string]any {
	typ := getString(m, "type")
	if typ == "" {
		return nil
	}
	obj, _ := m[typ].(map[string]any)
	return obj
}

// sortedKeys returns a map's keys in stable sorted order, so text
// rendered from a map is deterministic across runs.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func isOpen(set map[string]struct{}, id string) bool {
	_, ok := set[id]
	return ok
}

// isEmptyValue reports whether a decoded JSON value is "falsy" in the
// sense the property walker cares about — a set property whose innermost
// value is empty means the property is unset.
func isEmptyValue(v any) bool {
	switch t := v.(type) {
	case nil:
		return true
	case string:
		return t == ""
	case bool:
		return !t
	case float64:
		return t == 0
	case []any:
		return len(t) == 0
	case map[string]any:
		return len(t) == 0
	default:
		return false
	}
}

// stripDashes removes dashes from a UUID for use in Notion fragment
// links, which reference blocks by their dash-free id.
func stripDashes(id string) string {
	return strings.ReplaceAll(id, "-", "")
}

// parseNotionTime parses an ISO-8601 timestamp (e.g.
// "2026-01-01T00:00:00.000Z") into a UTC time. Returns nil on failure so
// callers can omit the field rather than record a zero time.
func parseNotionTime(s string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil
	}
	utc := t.UTC()
	return &utc
}
