package handler

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

func sessionEventSandboxID(event model.SessionEvent) *uuid.UUID {
	if event.SandboxID == nil {
		return nil
	}
	id := *event.SandboxID
	return &id
}

func rawScheduleEventPayload(payload model.JSON) model.RawJSON {
	if payload == nil {
		return model.RawJSON("{}")
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return model.RawJSON("{}")
	}
	return model.RawJSON(encoded)
}

func scheduleStatusFromEvent(eventType string, payload map[string]any) string {
	switch eventType {
	case "schedule.paused":
		return "paused"
	case "schedule.resumed", "schedule.created", "schedule.updated":
		return "active"
	case "schedule.cancelled":
		return "cancelled"
	}
	switch strings.ToLower(stringValue(payload, "state")) {
	case "paused":
		return "paused"
	case "completed":
		return "completed"
	default:
		return "active"
	}
}

func latestScheduleStatus(eventType string, payload map[string]any) string {
	switch eventType {
	case "schedule.run_started":
		return "running"
	case "schedule.run_completed":
		return "completed"
	case "schedule.run_failed":
		return "failed"
	default:
		return stringValue(payload, "last_status")
	}
}

func runStatusFromEvent(eventType string) string {
	switch eventType {
	case "schedule.run_completed":
		return "completed"
	case "schedule.run_failed":
		return "failed"
	default:
		return "running"
	}
}

func timePtrFromPayload(payload map[string]any, key string) *time.Time {
	value := stringValue(payload, key)
	if value == "" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return nil
	}
	utc := parsed.UTC()
	return &utc
}

func int64PtrFromPayload(payload map[string]any, key string) *int64 {
	if value, ok := payload[key]; ok && value != nil {
		n := int64FromAny(value)
		return &n
	}
	return nil
}

func int64Value(payload map[string]any, key string) int64 {
	if value, ok := payload[key]; ok && value != nil {
		return int64FromAny(value)
	}
	return 0
}

func int64FromAny(value any) int64 {
	switch typed := value.(type) {
	case float64:
		return int64(typed)
	case int64:
		return typed
	case int:
		return int64(typed)
	case json.Number:
		n, _ := typed.Int64()
		return n
	default:
		return 0
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
