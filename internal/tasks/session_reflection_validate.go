package tasks

import (
	"strings"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/runtimeevents"
)

// filterReflectionCandidatesByEvidence applies the deterministic half of the
// memory-quality policy. The extraction model proposes candidates, but it
// cannot make agent narration authoritative or invent valid provenance.
func filterReflectionCandidatesByEvidence(
	candidates []reflectionMemoryCandidate,
	events map[uuid.UUID]model.SessionEvent,
) []reflectionMemoryCandidate {
	kept := make([]reflectionMemoryCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		normalized, ok := validateReflectionCandidateEvidence(candidate, events)
		if ok {
			kept = append(kept, normalized)
		}
	}
	return kept
}

func validateReflectionCandidateEvidence(
	candidate reflectionMemoryCandidate,
	events map[uuid.UUID]model.SessionEvent,
) (reflectionMemoryCandidate, bool) {
	if len(events) == 0 || reflectionContentNarratesOperationalState(candidate.Content) {
		return reflectionMemoryCandidate{}, false
	}

	seen := make(map[uuid.UUID]bool, len(candidate.SourceEventIDs))
	humanIDs := make([]string, 0, len(candidate.SourceEventIDs))
	evidenceIDs := make([]string, 0, len(candidate.SourceEventIDs))
	agentIDs := make([]string, 0, len(candidate.SourceEventIDs))
	for _, raw := range candidate.SourceEventIDs {
		id, err := uuid.Parse(strings.TrimSpace(raw))
		if err != nil || seen[id] {
			continue
		}
		event, ok := events[id]
		if !ok {
			continue
		}
		seen[id] = true
		switch {
		case isReflectionHumanMessageEvent(event.EventType):
			humanIDs = append(humanIDs, id.String())
		case isReflectionAgentMessageEvent(event.EventType):
			agentIDs = append(agentIDs, id.String())
		case isReflectionDiagnosticEvidenceEvent(event.EventType):
			evidenceIDs = append(evidenceIDs, id.String())
		}
	}

	hasHuman := len(humanIDs) > 0
	switch candidate.Kind {
	case "finding", "workaround":
		// A human statement is sufficient. Without one, require both the raw
		// diagnostic evidence and the agent's explicit conclusion; neither a
		// command result nor agent self-report is durable evidence by itself.
		if !hasHuman && (len(evidenceIDs) == 0 || len(agentIDs) == 0) {
			return reflectionMemoryCandidate{}, false
		}
	default:
		if !hasHuman {
			return reflectionMemoryCandidate{}, false
		}
	}

	candidate.SourceEventIDs = append(humanIDs, evidenceIDs...)
	candidate.SourceEventIDs = append(candidate.SourceEventIDs, agentIDs...)
	if len(candidate.SourceEventIDs) == 0 {
		return reflectionMemoryCandidate{}, false
	}
	// Actor fields are derived from the cited human event at storage time.
	// Model-provided names are never trusted as provenance.
	candidate.ActorDisplayName = ""
	candidate.ActorExternalRef = ""
	return candidate, true
}

func isReflectionDiagnosticEvidenceEvent(eventType string) bool {
	switch eventType {
	case runtimeevents.EventToolResult, runtimeevents.EventToolCallCompleted,
		runtimeevents.EventError, runtimeevents.EventAgentError,
		runtimeevents.EventTurnFailed, runtimeevents.EventSubagentErrored:
		return true
	default:
		return false
	}
}

// reflectionContentNarratesOperationalState rejects the characteristic shape
// of capability probes and session recaps even when the model cites multiple
// events. Durable memories state the underlying fact directly.
func reflectionContentNarratesOperationalState(content string) bool {
	value := strings.ToLower(strings.Join(strings.Fields(content), " "))
	for _, marker := range []string{
		"the agent can ",
		"the agent could ",
		"the agent has access",
		"the agent was able",
		"the agent returned",
		"the agent reported",
		"the agent noted",
		"the agent posted",
		"the agent successfully",
		"agent posted a hello message",
		"agent can successfully",
		"tool catalog",
		"tools available",
		"available tools",
		"workspace tools",
		"public, non-archived slack channels",
		"posted a hello message",
		"limited result set",
	} {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return false
}
