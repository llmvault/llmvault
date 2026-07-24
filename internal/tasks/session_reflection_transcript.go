package tasks

import (
	"strings"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

const (
	reflectionTranscriptMaxText    = 1600
	reflectionTranscriptMaxSummary = 120
)

type reflectionIdentity struct {
	UserID      *uuid.UUID
	DisplayName string
	ExternalRef string
}

func renderSessionReflectionTranscript(session model.Session, channelName string, events []model.SessionEvent, userNames map[uuid.UUID]string) (string, map[uuid.UUID]reflectionIdentity) {
	identities := resolveReflectionIdentities(session, events, userNames)
	var b strings.Builder
	b.WriteString("# Session transcript\n\n")
	b.WriteString("Session ID: ")
	b.WriteString(session.ID.String())
	b.WriteString("\nAgent ID: ")
	b.WriteString(session.AgentID.String())
	b.WriteString("\nSource: ")
	b.WriteString(session.Source)
	// Session Date is the temporal anchor the extraction prompt resolves
	// relative dates against ("yesterday" → an absolute date).
	if len(events) > 0 {
		b.WriteString("\nSession Date: ")
		b.WriteString(events[len(events)-1].EventAt.UTC().Format("2006-01-02"))
	}
	if channelName = strings.TrimSpace(channelName); channelName != "" {
		b.WriteString("\nChannel: ")
		b.WriteString(channelName)
	}
	b.WriteString("\n\n")
	for _, event := range events {
		if !shouldRenderReflectionEvent(event.EventType) {
			continue
		}
		writeReflectionEvent(&b, event, identities[event.ID])
	}
	return strings.TrimSpace(b.String()), identities
}

func resolveReflectionIdentities(session model.Session, events []model.SessionEvent, userNames map[uuid.UUID]string) map[uuid.UUID]reflectionIdentity {
	out := make(map[uuid.UUID]reflectionIdentity, len(events))
	for _, event := range events {
		identity := reflectionIdentity{}
		// ActorUserID is also propagated as the authorization actor for an
		// entire turn, so it does not identify the speaker of agent/tool events.
		// Only inbound human events may carry a human identity into reflection.
		if !isReflectionHumanMessageEvent(event.EventType) {
			out[event.ID] = identity
			continue
		}
		if event.ActorUserID != nil {
			identity.UserID = event.ActorUserID
			identity.DisplayName = firstNonEmptyString(userNames[*event.ActorUserID], event.ActorUserID.String())
		} else if session.Source != model.SessionSourceExternal && session.CreatedBy != nil {
			identity.UserID = session.CreatedBy
			identity.DisplayName = firstNonEmptyString(userNames[*session.CreatedBy], session.CreatedBy.String())
		}
		if slack := payloadMap(event.Payload, "slack"); slack != nil {
			sender := payloadString(slack, "sender_id")
			if sender != "" {
				identity.ExternalRef = "<@" + sender + ">"
				identity.DisplayName = firstNonEmptyString(
					payloadString(slack, "display_name"),
					payloadString(slack, "user_name"),
					payloadString(slack, "sender_tag"),
					identity.ExternalRef,
				)
			}
		}
		out[event.ID] = identity
	}
	return out
}

func writeReflectionEvent(b *strings.Builder, event model.SessionEvent, identity reflectionIdentity) {
	b.WriteString("## ")
	b.WriteString(event.EventAt.UTC().Format("2006-01-02 15:04:05 UTC"))
	b.WriteString(" [event:")
	b.WriteString(event.ID.String())
	b.WriteString("]\n")
	b.WriteString("Type: ")
	b.WriteString(event.EventType)
	b.WriteString("\n")
	b.WriteString("Role: ")
	b.WriteString(reflectionEventRole(event.EventType))
	b.WriteString("\n")
	if event.TurnID != "" {
		b.WriteString("Turn: ")
		b.WriteString(event.TurnID)
		b.WriteString("\n")
	}
	if identity.DisplayName != "" || identity.ExternalRef != "" || identity.UserID != nil {
		b.WriteString("Actor: ")
		b.WriteString(formatReflectionActor(identity))
		b.WriteString("\n")
	}
	if slack := payloadMap(event.Payload, "slack"); slack != nil && isReflectionHumanMessageEvent(event.EventType) {
		b.WriteString("Slack: ")
		b.WriteString(formatSlackReflectionContext(slack))
		b.WriteString("\n")
	}
	if isReflectionMessageEvent(event.EventType) {
		if text := reflectionEventText(event); text != "" {
			b.WriteString("\n")
			b.WriteString(text)
			b.WriteString("\n")
		}
	} else if summary := reflectionEventSummary(event); summary != "" {
		b.WriteString("\n")
		b.WriteString(summary)
		b.WriteString("\n")
	}
	b.WriteString("\n")
}
