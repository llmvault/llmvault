package model

import "strings"

// ValidReasoningEfforts is the canonical set of reasoning effort levels shared
// by sessions and agent-level defaults. Empty means "unset".
var ValidReasoningEfforts = []string{"low", "medium", "high"}

// NormalizeReasoningEffort lower-cases and trims a reasoning effort value.
// It returns ("", true) for an empty value (unset) and (value, true) for a
// recognized level. Unknown values return ("", false).
func NormalizeReasoningEffort(value string) (string, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "", true
	}
	switch value {
	case "low", "medium", "high":
		return value, true
	default:
		return "", false
	}
}
