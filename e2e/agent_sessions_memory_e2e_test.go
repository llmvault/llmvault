package e2e

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

const (
	agentSessionsMemoryProvider = "linear"
	agentSessionsMemoryType     = "technical_context"
	agentSessionsRetainTool     = "hivy_memory_retain"
	agentSessionsRecallTool     = "hivy_memory_recall"
)

type agentSessionsMemoryFact struct {
	Key   string
	Value string
}

func requireAgentSessionsHindsightHealthy(t *testing.T, ctx context.Context) {
	t.Helper()
	port := strings.TrimSpace(os.Getenv("HIVY_COMPOSE_HINDSIGHT_API_PORT"))
	if port == "" {
		port = "8888"
	}
	url := "http://localhost:" + port + "/health"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("build Hindsight health request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("hindsight at %s is not reachable; start the compose stack first: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("hindsight health status=%d body=%s", resp.StatusCode, body)
	}
}

func runAgentSessionsHindsightMemoryE2E(t *testing.T, ctx context.Context, apiBase, ownerToken, orgID, channelID, runID string) {
	t.Helper()

	facts := agentSessionsMemoryFacts(runID)
	retainMarker := "HINDSIGHT_E2E_RETAINED_" + runID
	recallMarker := "HINDSIGHT_E2E_RECALLED_" + runID

	retainSession := agentSessionsCreateSessionWithPayload(t, ctx, apiBase, ownerToken, orgID, map[string]any{
		"channel_id":       channelID,
		"reasoning_effort": "low",
		"text":             agentSessionsMemoryRetainPrompt(runID, retainMarker, facts),
	})
	if retainSession.Session.ID == "" || retainSession.Event == nil {
		t.Fatalf("memory retain session was not created correctly: %+v", retainSession)
	}
	t.Logf("created Hindsight retain session id=%s queued=%t", retainSession.Session.ID, retainSession.Queued)

	retainAccess := fetchAgentSessionsSandboxAccess(t, ctx, apiBase, ownerToken, orgID, retainSession.Session.ID)
	retainStream := agentSessionsStartDirectStream(t, ctx, agentSessionsDirectStreamURL(retainAccess, retainSession.Session.ID), retainAccess.Token)
	retainEvents := retainStream.collectUntil(t, ctx, 5*time.Minute, func(event runtimeSSEEvent) bool {
		return event.Name == "final" && strings.Contains(event.RawData, retainMarker)
	})
	assertAgentSessionsMemoryRetainEvents(t, retainEvents, facts)
	waitForAgentSessionsResponse(t, ctx, apiBase, ownerToken, orgID, retainSession.Session.ID, retainMarker)

	t.Logf("waiting 30s for async Hindsight retain processing")
	select {
	case <-ctx.Done():
		t.Fatalf("context expired while waiting for Hindsight retain processing: %v", ctx.Err())
	case <-time.After(30 * time.Second):
	}

	recallSession := agentSessionsCreateSessionWithPayload(t, ctx, apiBase, ownerToken, orgID, map[string]any{
		"channel_id":       channelID,
		"reasoning_effort": "low",
		"text":             agentSessionsMemoryRecallPrompt(runID, recallMarker, facts),
	})
	if recallSession.Session.ID == "" || recallSession.Event == nil {
		t.Fatalf("memory recall session was not created correctly: %+v", recallSession)
	}
	t.Logf("created Hindsight recall session id=%s queued=%t", recallSession.Session.ID, recallSession.Queued)

	recallAccess := fetchAgentSessionsSandboxAccess(t, ctx, apiBase, ownerToken, orgID, recallSession.Session.ID)
	recallStream := agentSessionsStartDirectStream(t, ctx, agentSessionsDirectStreamURL(recallAccess, recallSession.Session.ID), recallAccess.Token)
	recallEvents := recallStream.collectUntil(t, ctx, 5*time.Minute, func(event runtimeSSEEvent) bool {
		return event.Name == "final" && strings.Contains(event.RawData, recallMarker)
	})
	assertAgentSessionsMemoryRecallEvents(t, recallEvents, facts)
	recallResponse := waitForAgentSessionsResponse(t, ctx, apiBase, ownerToken, orgID, recallSession.Session.ID, recallMarker)
	assertAgentSessionsMemoryFinalPayload(t, recallResponse.Payload, facts)
}

func agentSessionsMemoryFacts(runID string) []agentSessionsMemoryFact {
	prefix := "HINDSIGHT_E2E_MEMORY_" + runID
	return []agentSessionsMemoryFact{
		{
			Key:   prefix + "_ALPHA",
			Value: "amber-harbor-" + runID,
		},
		{
			Key:   prefix + "_BRAVO",
			Value: "cedar-bridge-" + runID,
		},
		{
			Key:   prefix + "_CHARLIE",
			Value: "violet-compass-" + runID,
		},
		{
			Key:   prefix + "_DELTA",
			Value: "silver-lantern-" + runID,
		},
	}
}

func agentSessionsMemoryRetainPrompt(runID, marker string, facts []agentSessionsMemoryFact) string {
	lines := []string{
		"This is the flagship Hindsight memory retain E2E.",
		"Call the real hivy_memory_retain MCP tool once for each memory below before replying.",
		"Use these exact tags for every memory: {\"scope\":\"provider\",\"provider\":\"" + agentSessionsMemoryProvider + "\",\"memory_type\":\"" + agentSessionsMemoryType + "\"}.",
		"Use this context for every memory: Hivy flagship E2E memory retention marker " + runID + ".",
		"Do not call memory_recall or memory_reflect in this turn.",
		"Store each of these as a separate exact factual statement:",
	}
	for i, fact := range facts {
		lines = append(lines, fmt.Sprintf("%d. %s", i+1, agentSessionsMemoryFactContent(fact)))
	}
	lines = append(lines,
		"After every memory_retain tool result succeeds, reply exactly "+marker+" and no other text.",
	)
	return strings.Join(lines, "\n")
}

func agentSessionsMemoryRecallPrompt(runID, marker string, facts []agentSessionsMemoryFact) string {
	keys := make([]string, 0, len(facts))
	for _, fact := range facts {
		keys = append(keys, fact.Key)
	}
	return strings.Join([]string{
		"This is the flagship Hindsight memory recall E2E.",
		"The values are intentionally not included in this prompt.",
		"Before replying, call the real hivy_memory_recall MCP tool exactly once.",
		"Use these exact tags: {\"scope\":\"provider\",\"provider\":\"" + agentSessionsMemoryProvider + "\",\"memory_type\":\"" + agentSessionsMemoryType + "\"}.",
		"Use budget \"high\" and query: Recall the exact values for these Hindsight E2E memory keys from the previous session: " + strings.Join(keys, ", ") + ".",
		"After memory_recall returns, reply with " + marker + " followed by one compact JSON object mapping each key to its recalled value.",
		"Do not call memory_retain or memory_reflect in this turn.",
		"Run marker: " + runID + ".",
	}, "\n")
}

func agentSessionsMemoryFactContent(fact agentSessionsMemoryFact) string {
	return fact.Key + " has exact value " + fact.Value + "."
}

func (s *agentSessionsLiveDirectStream) collectUntil(t *testing.T, ctx context.Context, timeout time.Duration, done func(runtimeSSEEvent) bool) []runtimeSSEEvent {
	t.Helper()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	var events []runtimeSSEEvent
	for {
		select {
		case event := <-s.events:
			events = append(events, event)
			if done(event) {
				return events
			}
		case completed := <-s.done:
			events = append(events, completed...)
			t.Fatalf("direct stream completed before expected event; events=%s", summarizeRuntimeSSEEvents(events))
		case err := <-s.errs:
			t.Fatalf("direct stream failed before expected event: %v; events=%s", err, summarizeRuntimeSSEEvents(events))
		case <-ctx.Done():
			t.Fatalf("direct stream context ended before expected event: %v; events=%s", ctx.Err(), summarizeRuntimeSSEEvents(events))
		case <-timer.C:
			t.Fatalf("timed out waiting for direct stream event; events=%s", summarizeRuntimeSSEEvents(events))
		}
	}
}
