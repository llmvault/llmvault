package tasks

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// payloadText reads a string scalar from a webhook payload at a dot-path.
func payloadText(wp map[string]any, path string) string {
	value, ok := lookupTriggerPayloadPath(wp, path)
	if !ok {
		return ""
	}
	return scalarString(value)
}

// payloadNumber formats a numeric payload field as an integer string. JSON
// numbers decode to float64, whose default formatting switches to scientific
// notation for large ids — so ids are formatted explicitly here.
func payloadNumber(wp map[string]any, path string) string {
	value, ok := lookupTriggerPayloadPath(wp, path)
	if !ok {
		return ""
	}
	return payloadNumberValue(value)
}

func payloadNumberValue(value any) string {
	switch typed := value.(type) {
	case float64:
		return strconv.FormatInt(int64(typed), 10)
	case json.Number:
		return typed.String()
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case string:
		return strings.TrimSpace(typed)
	case nil:
		return ""
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}
