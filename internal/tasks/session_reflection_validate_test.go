package tasks

import (
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/runtimeevents"
)

func TestFilterReflectionCandidatesRejectsCapabilityAndInventoryMemories(t *testing.T) {
	humanID := uuid.New()
	toolID := uuid.New()
	agentID := uuid.New()
	events := map[uuid.UUID]model.SessionEvent{
		humanID: {ID: humanID, EventType: runtimeevents.EventUserMessageReceived},
		toolID:  {ID: toolID, EventType: runtimeevents.EventToolResult},
		agentID: {ID: agentID, EventType: runtimeevents.EventFinal},
	}
	candidates := []reflectionMemoryCandidate{
		{
			Content:        "The agent could see these public, non-archived Slack channels: all-hive, social, engineering, and qa.",
			Kind:           "org-fact",
			SourceEventIDs: []string{humanID.String()},
		},
		{
			Content:        "The workspace tools available are bash, read_file, search_sessions, and update_plan.",
			Kind:           "finding",
			SourceEventIDs: []string{toolID.String(), agentID.String()},
		},
		{
			Content:        "The hello message proved the agent can successfully post messages to Slack.",
			Kind:           "finding",
			SourceEventIDs: []string{toolID.String(), agentID.String()},
		},
		{
			Content:        "On July 16, 2026 the Hivy agent posted a hello message to #all-hive, showing the agent can successfully post messages to Slack channels.",
			Kind:           "org-fact",
			SourceEventIDs: []string{humanID.String(), toolID.String(), agentID.String()},
		},
	}

	if kept := filterReflectionCandidatesByEvidence(candidates, events); len(kept) != 0 {
		t.Fatalf("operational memories survived evidence filtering: %#v", kept)
	}
}

func TestFilterReflectionCandidatesRequiresAuthoritativeEvidence(t *testing.T) {
	humanID := uuid.New()
	toolID := uuid.New()
	agentID := uuid.New()
	events := map[uuid.UUID]model.SessionEvent{
		humanID: {ID: humanID, EventType: runtimeevents.EventUserMessageReceived},
		toolID:  {ID: toolID, EventType: runtimeevents.EventToolResult},
		agentID: {ID: agentID, EventType: runtimeevents.EventFinal},
	}
	candidates := []reflectionMemoryCandidate{
		{
			Content:          "Dana requires a second engineer to review every database migration before it ships.",
			Kind:             "rule",
			SourceEventIDs:   []string{humanID.String(), uuid.NewString()},
			ActorDisplayName: "Invented Name",
		},
		{
			Content:        "Production uses Railway.",
			Kind:           "org-fact",
			SourceEventIDs: []string{toolID.String(), agentID.String()},
		},
		{
			Content:        "Deployments fail above a 1 MiB manifest; pruning source maps keeps releases below the limit.",
			Kind:           "workaround",
			SourceEventIDs: []string{toolID.String(), agentID.String()},
		},
		{
			Content:        "The repository uses Go.",
			Kind:           "org-fact",
			SourceEventIDs: []string{uuid.NewString()},
		},
	}

	kept := filterReflectionCandidatesByEvidence(candidates, events)
	if len(kept) != 2 {
		t.Fatalf("kept %d candidates, want rule and verified workaround: %#v", len(kept), kept)
	}
	if kept[0].Kind != "rule" || len(kept[0].SourceEventIDs) != 1 || kept[0].SourceEventIDs[0] != humanID.String() {
		t.Fatalf("human rule provenance=%#v", kept[0])
	}
	if kept[0].ActorDisplayName != "" || kept[0].ActorExternalRef != "" {
		t.Fatalf("model-provided actor fields survived: %#v", kept[0])
	}
	if kept[1].Kind != "workaround" || len(kept[1].SourceEventIDs) != 2 {
		t.Fatalf("verified workaround provenance=%#v", kept[1])
	}
}
