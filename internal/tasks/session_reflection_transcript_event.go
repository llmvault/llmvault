package tasks

import (
	"fmt"
	"sort"
	"strings"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/runtimeevents"
)

func shouldRenderReflectionEvent(eventType string) bool {
	switch eventType {
	case runtimeevents.EventThinking, runtimeevents.EventReasoningDelta, runtimeevents.EventReasoningStarted,
		runtimeevents.EventReasoningCompleted, runtimeevents.EventToken, runtimeevents.EventResponseChunk,
		runtimeevents.EventModelUsage:
		return false
	default:
		return true
	}
}

// isReflectionMessageEvent reports whether the event carries a human or agent
// message whose full text belongs in the transcript. Everything else (tool
// results, lifecycle events) is reduced to a one-line summary — the junk
// memories provably originate in raw tool output.
func isReflectionMessageEvent(eventType string) bool {
	return isReflectionHumanMessageEvent(eventType) || isReflectionAgentMessageEvent(eventType)
}

func isReflectionHumanMessageEvent(eventType string) bool {
	switch eventType {
	case runtimeevents.EventUserMessageReceived, runtimeevents.EventMessageReceived,
		runtimeevents.EventQuestionAnswered:
		return true
	default:
		return false
	}
}

func isReflectionAgentMessageEvent(eventType string) bool {
	switch eventType {
	case runtimeevents.EventFinal, runtimeevents.EventResponseCompleted,
		runtimeevents.EventQuestionRequested:
		return true
	default:
		return false
	}
}

func reflectionEventRole(eventType string) string {
	switch {
	case isReflectionHumanMessageEvent(eventType):
		return "Human"
	case isReflectionAgentMessageEvent(eventType):
		return "Agent"
	default:
		return "Tool/Event"
	}
}

// reflectionEventSummary renders a non-message event as a single line:
// tool/event name, ok/error, and the first ~120 characters of its output.
func reflectionEventSummary(event model.SessionEvent) string {
	name := firstNonEmptyString(
		payloadString(event.Payload, "tool_name"),
		payloadString(event.Payload, "tool"),
		payloadString(event.Payload, "name"),
		event.EventType,
	)
	errText := strings.TrimSpace(payloadString(event.Payload, "error"))
	status := "ok"
	if errText != "" || isReflectionErrorEventType(event.EventType) ||
		payloadString(event.Payload, "status") == "error" || payloadString(event.Payload, "status") == "failed" {
		status = "error"
	}
	preview := errText
	if preview == "" {
		for _, key := range []string{"text", "content", "message", "result", "output", "summary"} {
			if value := strings.TrimSpace(payloadString(event.Payload, key)); value != "" {
				preview = value
				break
			}
		}
	}
	if preview == "" {
		preview = formatSmallPayload(event.Payload)
	}
	line := "Result: " + name + " " + status
	if preview = trimReflectionTextTo(preview, reflectionTranscriptMaxSummary); preview != "" {
		line += " — " + preview
	}
	return line
}

func isReflectionErrorEventType(eventType string) bool {
	switch eventType {
	case runtimeevents.EventError, runtimeevents.EventAgentError, runtimeevents.EventTurnFailed,
		runtimeevents.EventSubagentErrored:
		return true
	default:
		return false
	}
}

func reflectionEventText(event model.SessionEvent) string {
	for _, key := range []string{"text", "content", "message", "error"} {
		if value := strings.TrimSpace(payloadString(event.Payload, key)); value != "" {
			return trimReflectionText(value)
		}
	}
	if usage := payloadMap(event.Payload, "usage"); usage != nil {
		return ""
	}
	return trimReflectionText(formatSmallPayload(event.Payload))
}

func formatReflectionActor(identity reflectionIdentity) string {
	parts := make([]string, 0, 3)
	if identity.DisplayName != "" {
		parts = append(parts, identity.DisplayName)
	}
	if identity.UserID != nil {
		parts = append(parts, "hivy_user="+identity.UserID.String())
	}
	if identity.ExternalRef != "" && identity.ExternalRef != identity.DisplayName {
		parts = append(parts, "external="+identity.ExternalRef)
	}
	return strings.Join(parts, " ")
}

func formatSlackReflectionContext(slack map[string]any) string {
	keys := []string{"team_id", "channel_id", "thread_ts", "message_ts", "sender_tag", "user_name", "display_name"}
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		if value := payloadString(slack, key); value != "" {
			parts = append(parts, key+"="+value)
		}
	}
	sort.Strings(parts)
	return strings.Join(parts, " ")
}

func payloadMap(payload map[string]any, key string) map[string]any {
	raw, ok := payload[key]
	if !ok {
		return nil
	}
	switch value := raw.(type) {
	case map[string]any:
		return value
	case model.JSON:
		return map[string]any(value)
	default:
		return nil
	}
}

func payloadString(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	value, ok := payload[key]
	if !ok || value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case fmt.Stringer:
		return strings.TrimSpace(v.String())
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}

func formatSmallPayload(payload model.JSON) string {
	parts := make([]string, 0, len(payload))
	for key, value := range payload {
		if key == "slack" || key == "usage" {
			continue
		}
		text := strings.TrimSpace(fmt.Sprint(value))
		if text != "" {
			parts = append(parts, key+"="+text)
		}
	}
	sort.Strings(parts)
	return strings.Join(parts, "\n")
}

func trimReflectionText(value string) string {
	return trimReflectionTextTo(value, reflectionTranscriptMaxText)
}

func trimReflectionTextTo(value string, max int) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if len(value) > max {
		value = strings.TrimSpace(value[:max]) + "..."
	}
	return value
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
