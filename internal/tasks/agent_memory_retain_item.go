package tasks

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/hindsight"
	"github.com/usehivy/hivy/internal/model"
)

func buildAgentRetainItem(agent *model.Agent, payload AgentMemoryRetainPayload, events []model.SessionEvent) (hindsight.RetainItem, bool) {
	item, ok, _ := buildAgentRetainItemWithReason(agent, payload, events)
	return item, ok
}

func buildAgentRetainItemWithReason(agent *model.Agent, payload AgentMemoryRetainPayload, events []model.SessionEvent) (hindsight.RetainItem, bool, string) {
	if agent == nil || agent.OrgID == nil || len(events) == 0 {
		return hindsight.RetainItem{}, false, "missing_context_or_events"
	}
	if sessionEventsContainSecret(events) {
		return hindsight.RetainItem{}, false, "secret_detected"
	}
	digest := agentMemoryRetentionDigest(agent.Name, events)
	if !meaningfulAgentMemoryTranscript(digest, events) {
		return hindsight.RetainItem{}, false, "not_meaningful"
	}
	source := dominantAgentMemorySource(events)
	tags := agentMemoryTags(agent, source)
	channel := firstAgentPayloadString(events, "channel")
	if channel != "" {
		tags = append(tags, "channel:"+sanitizeMemoryTagValue(channel))
	}
	observationScopes := [][]string{{"company:" + agent.OrgID.String()}}
	return hindsight.RetainItem{
		Content:           digest,
		Context:           agentMemoryRetentionContext(source),
		DocumentID:        agentMemoryDocumentID(payload),
		Tags:              tags,
		Timestamp:         events[0].EventAt.UTC().Format(time.RFC3339),
		Metadata:          agentMemoryRetainMetadata(agent, payload, events),
		ObservationScopes: observationScopes,
	}, true, ""
}

func agentMemoryRetentionContext(source string) string {
	return fmt.Sprintf("Settled agent session transcript from %s source. It includes user and agent messages only. Retain only information that will be useful in future work: stable people facts, stable channel user IDs or mention handles when present, company facts, decisions, preferences, ownership, policies, recurring workflows, and reusable technical context. If the transcript is only social chatter, jokes, acknowledgements, temporary status, active task framing, or ordinary completion chatter, retain nothing. Never retain secrets or pasted credential values.", source)
}

func agentMemoryDocumentID(payload AgentMemoryRetainPayload) string {
	if payload.SessionUUID != uuid.Nil {
		return "session:" + payload.SessionUUID.String()
	}
	return "session:" + payload.SandboxID.String() + ":" + payload.SessionID
}

func agentMemoryRetentionDigest(agentName string, events []model.SessionEvent) string {
	var lines []string
	for _, event := range events {
		payload := agentMemoryPayload(event)
		switch event.EventType {
		case "user.message.received":
			speaker := agentMemorySpeaker(payload)
			if speaker == "" {
				speaker = "teammate"
			}
			text := firstPayloadString(payload, "text", "message")
			if shouldIncludeAgentMemoryMessage(text) {
				lines = append(lines, fmt.Sprintf("Teammate %s: %s", speaker, text))
			}
		case "final":
			text := firstPayloadString(payload, "text", "message")
			if shouldIncludeAgentMemoryMessage(text) {
				lines = append(lines, fmt.Sprintf("Agent %s: %s", agentName, text))
			}
		}
	}
	if len(lines) == 0 {
		return ""
	}
	var buf strings.Builder
	buf.WriteString("Settled session transcript for memory extraction. This omits raw tool calls, internal commands, execution trace, and streamed partial output.\n\n")
	for _, line := range lines {
		buf.WriteString("- ")
		buf.WriteString(line)
		buf.WriteString("\n")
	}
	return strings.TrimSpace(buf.String())
}

func agentMemorySpeaker(payload map[string]any) string {
	name := firstPayloadString(payload, "user_display_name")
	userID := firstPayloadString(payload, "user")
	mention := agentMemorySlackMention(userID)
	switch {
	case name != "" && mention != "":
		return fmt.Sprintf("%s (%s)", name, mention)
	case name != "":
		return name
	case mention != "":
		return mention
	default:
		return userID
	}
}

func agentMemorySlackMention(userID string) string {
	userID = strings.TrimSpace(userID)
	if strings.HasPrefix(userID, "U") || strings.HasPrefix(userID, "W") {
		return "<@" + userID + ">"
	}
	return ""
}

func agentMemoryRetainMetadata(agent *model.Agent, payload AgentMemoryRetainPayload, events []model.SessionEvent) map[string]string {
	meta := map[string]string{
		"agent_id":     agent.ID.String(),
		"sandbox_id":   payload.SandboxID.String(),
		"session_uuid": payload.SessionUUID.String(),
		"session_id":   payload.SessionID,
		"event_count":  fmt.Sprintf("%d", len(events)),
		"source_event": payload.SourceEvent,
	}
	for _, key := range []string{"source", "channel", "thread_ts", "user", "user_display_name", "tool"} {
		if value := firstAgentPayloadString(events, key); value != "" {
			meta[key] = value
		}
	}
	return meta
}

func firstAgentPayloadString(events []model.SessionEvent, key string) string {
	for _, event := range events {
		if value := firstPayloadString(agentMemoryPayload(event), key); value != "" {
			return value
		}
	}
	return ""
}

func firstPayloadString(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := payload[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func sessionEventIDs(events []model.SessionEvent) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(events))
	for _, event := range events {
		ids = append(ids, event.ID)
	}
	return ids
}
