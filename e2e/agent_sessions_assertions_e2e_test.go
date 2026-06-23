package e2e

import (
	"fmt"
	"strings"
	"testing"
)

func findDefaultAgent(t *testing.T, agents []agentSessionsAgentListItem) agentSessionsAgentListItem {
	t.Helper()
	for _, agent := range agents {
		if agent.IsDefault {
			return agent
		}
	}
	t.Fatalf("default agent not found: %+v", agents)
	return agentSessionsAgentListItem{}
}

func findDefaultGeneralChannel(t *testing.T, channels []agentSessionsChannel, agentID string) agentSessionsChannel {
	t.Helper()
	for _, channel := range channels {
		if channel.Name == "general" && channel.IsDefault {
			if channel.DefaultAgentID != agentID {
				t.Fatalf("#general default_agent_id=%s want %s", channel.DefaultAgentID, agentID)
			}
			return channel
		}
	}
	t.Fatalf("default #general channel not found: %+v", channels)
	return agentSessionsChannel{}
}

func assertAgentSessionsParticipant(t *testing.T, detail agentSessionsDetail, userID string) {
	t.Helper()
	for _, participant := range detail.Participants {
		if participant.UserID == userID {
			if participant.Role == "" || participant.JoinedAt == nil || *participant.JoinedAt == "" {
				t.Fatalf("participant was not auto-joined: %+v", participant)
			}
			return
		}
	}
	t.Fatalf("participant %s not found in detail: %+v", userID, detail.Participants)
}

func assertAgentSessionsEventOrder(t *testing.T, events []agentSessionsEvent) {
	t.Helper()
	var agentMessages, participantJoins int
	for _, event := range events {
		switch event.EventType {
		case "final":
			agentMessages++
		case "participant.joined":
			participantJoins++
		}
	}
	if agentMessages < 2 || participantJoins < 1 {
		t.Fatalf("session events missing expected multiplayer history: agent=%d joins=%d events=%+v", agentMessages, participantJoins, events)
	}
}

func assertAgentSessionsBackendOwnedUserMessages(t *testing.T, events []agentSessionsEvent, userIDs ...string) {
	t.Helper()
	want := map[string]bool{}
	for _, userID := range userIDs {
		if strings.TrimSpace(userID) != "" {
			want[userID] = true
		}
	}
	seen := map[string]bool{}
	var candidates []agentSessionsEvent
	for _, event := range events {
		if event.ActorUserID == nil || *event.ActorUserID == "" {
			continue
		}
		if !want[*event.ActorUserID] {
			continue
		}
		if event.RuntimeSeq != nil || strings.EqualFold(event.Source, "runtime") {
			continue
		}
		if sessionEventPayloadText(event.Payload) == "" {
			continue
		}
		candidates = append(candidates, event)
		seen[*event.ActorUserID] = true
	}
	for userID := range want {
		if !seen[userID] {
			t.Fatalf("backend-owned user message for actor %s not found; candidates=%+v events=%+v", userID, candidates, events)
		}
	}
}

func assertAgentSessionsBackendOwnedMutationEvent(t *testing.T, event *agentSessionsEvent) {
	t.Helper()
	if err := validateAgentSessionsBackendOwnedMutationEvent(event); err != nil {
		t.Fatal(err)
	}
}

func validateAgentSessionsBackendOwnedMutationEvent(event *agentSessionsEvent) error {
	if event == nil {
		return fmt.Errorf("mutation response missing backend-owned user event")
	}
	if event.EventType != "user.message.received" {
		return fmt.Errorf("mutation event type=%s, want user.message.received: %+v", event.EventType, event)
	}
	if event.RuntimeSeq != nil || strings.EqualFold(event.Source, "runtime") {
		return fmt.Errorf("mutation event should be backend-owned, got source=%s runtime_seq=%v event=%+v", event.Source, event.RuntimeSeq, event)
	}
	if sessionEventPayloadText(event.Payload) == "" {
		return fmt.Errorf("mutation event missing user text payload: %+v", event)
	}
	return nil
}

func sessionEventPayloadText(payload map[string]any) string {
	for _, key := range []string{"text", "message", "content"} {
		if value, _ := payload[key].(string); strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	if message, _ := payload["message"].(map[string]any); message != nil {
		for _, key := range []string{"text", "content"} {
			if value, _ := message[key].(string); strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
		}
	}
	return ""
}

func eventType(event *agentSessionsEvent) string {
	if event == nil {
		return ""
	}
	return event.EventType
}
