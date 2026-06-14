package e2e

import (
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
	var userMessages, agentMessages, participantJoins int
	userSequences := map[int64]bool{}
	joinSequences := map[int64]bool{}
	for _, event := range events {
		switch event.EventType {
		case "user.message":
			userMessages++
			userSequences[event.SequenceNumber] = true
		case "agent.message.sent":
			agentMessages++
		case "participant.joined":
			participantJoins++
			joinSequences[event.SequenceNumber] = true
		}
	}
	if userMessages < 2 || agentMessages < 2 || participantJoins < 1 {
		t.Fatalf("session events missing expected multiplayer flow: user=%d agent=%d joins=%d events=%+v", userMessages, agentMessages, participantJoins, events)
	}
	if !userSequences[1] || !joinSequences[2] || !userSequences[3] {
		t.Fatalf("session durable events missing expected queue sequence: user=%v joins=%v events=%+v", userSequences, joinSequences, events)
	}
}

func eventType(event *agentSessionsEvent) string {
	if event == nil {
		return ""
	}
	return event.EventType
}

func looksLikeDockerContainerID(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, r := range value {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}
