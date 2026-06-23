package handler

import (
	"strings"
	"time"
)

func formatRuntimeTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func formatRuntimeTimePtr(t *time.Time) *string {
	if t == nil || t.IsZero() {
		return nil
	}
	formatted := formatRuntimeTime(*t)
	return &formatted
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func webSessionName(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return "New session"
	}
	const max = 80
	if len(text) <= max {
		return text
	}
	return strings.TrimSpace(text[:max])
}
