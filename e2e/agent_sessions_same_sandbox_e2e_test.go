package e2e

import (
	"context"
	"strings"
	"testing"
	"time"
)

func runAgentSessionsSameSandboxDirectStreamsE2E(t *testing.T, ctx context.Context, apiBase, ownerToken, orgID, channelID, runID, sandboxID, sandboxBaseURL string) {
	t.Helper()

	slowToolMarker := "AGENT_SESSIONS_E2E_SAME_SANDBOX_SLOW_TOOL_" + runID
	fastToolMarker := "AGENT_SESSIONS_E2E_SAME_SANDBOX_FAST_TOOL_" + runID
	slowFinalMarker := "AGENT_SESSIONS_E2E_SAME_SANDBOX_SLOW_FINAL_" + runID
	fastFinalMarker := "AGENT_SESSIONS_E2E_SAME_SANDBOX_FAST_FINAL_" + runID

	slowSession := agentSessionsCreateSessionWithPayload(t, ctx, apiBase, ownerToken, orgID, map[string]any{
		"channel_id":       channelID,
		"model_definition": map[string]any{"reasoning_effort": "low"},
		"text": strings.Join([]string{
			"This is the same-sandbox direct stream E2E slow session.",
			"Before replying, call bash exactly once with this command: python3 -c 'import time; time.sleep(45); print(\"" + slowToolMarker + "\")'.",
			"After the bash result, visible final reply exactly " + slowFinalMarker + " and no other text.",
		}, "\n"),
	})
	if slowSession.Session.ID == "" {
		t.Fatalf("slow same-sandbox session was not created correctly: %+v", slowSession)
	}
	assertAgentSessionsBackendOwnedMutationEvent(t, slowSession.Event)
	slowAccess := fetchAgentSessionsSandboxAccess(t, ctx, apiBase, ownerToken, orgID, slowSession.Session.ID)
	assertAgentSessionsSameSandboxAccess(t, slowAccess, slowSession.Session.ID, sandboxID, sandboxBaseURL)
	slowStream := agentSessionsStartSandboxStreamWithAccess(t, ctx, slowSession.Session.ID, slowAccess)
	slowToolEvent := slowStream.waitForEvent(t, ctx, 2*time.Minute, func(event runtimeSSEEvent) bool {
		return event.Name == "tool_call" && strings.Contains(event.RawData, slowToolMarker)
	})
	t.Logf("same-sandbox slow session entered tool execution event=%s", slowToolEvent.RawData)

	fastSession := agentSessionsCreateSessionWithPayload(t, ctx, apiBase, ownerToken, orgID, map[string]any{
		"channel_id":       channelID,
		"model_definition": map[string]any{"reasoning_effort": "low"},
		"text": strings.Join([]string{
			"This is the same-sandbox direct stream E2E fast session.",
			"Before replying, call bash exactly once with this command: python3 -c 'print(\"" + fastToolMarker + "\")'.",
			"After the bash result, visible final reply exactly " + fastFinalMarker + " and no other text.",
		}, "\n"),
	})
	if fastSession.Session.ID == "" {
		t.Fatalf("fast same-sandbox session was not created correctly: %+v", fastSession)
	}
	assertAgentSessionsBackendOwnedMutationEvent(t, fastSession.Event)
	fastAccess := fetchAgentSessionsSandboxAccess(t, ctx, apiBase, ownerToken, orgID, fastSession.Session.ID)
	assertAgentSessionsSameSandboxAccess(t, fastAccess, fastSession.Session.ID, sandboxID, sandboxBaseURL)
	fastStream := agentSessionsStartSandboxStreamWithAccess(t, ctx, fastSession.Session.ID, fastAccess)

	fastToolEvent := fastStream.waitForEvent(t, ctx, 2*time.Minute, func(event runtimeSSEEvent) bool {
		return event.Name == "tool_call" && strings.Contains(event.RawData, fastToolMarker)
	})
	t.Logf("same-sandbox fast session entered tool execution while slow session was active event=%s", fastToolEvent.RawData)
	slowStream.assertNoBufferedEvent(t, func(event runtimeSSEEvent) bool {
		return strings.Contains(agentSessionsToolResultOutput(event), slowToolMarker) ||
			strings.Contains(event.RawData, slowFinalMarker)
	})
	fastFinalEvent := fastStream.waitForEvent(t, ctx, 3*time.Minute, func(event runtimeSSEEvent) bool {
		return strings.Contains(event.RawData, fastFinalMarker)
	})
	t.Logf("same-sandbox fast final observed on direct sandbox stream event=%s", fastFinalEvent.RawData)
	waitForAgentSessionsResponse(t, ctx, apiBase, ownerToken, orgID, fastSession.Session.ID, fastFinalMarker)

	slowFinalEvent := slowStream.waitForEvent(t, ctx, 3*time.Minute, func(event runtimeSSEEvent) bool {
		return strings.Contains(event.RawData, slowFinalMarker)
	})
	t.Logf("same-sandbox slow final observed on direct sandbox stream event=%s", slowFinalEvent.RawData)
	waitForAgentSessionsResponse(t, ctx, apiBase, ownerToken, orgID, slowSession.Session.ID, slowFinalMarker)

	fastEvents := agentSessionsListAllEvents(t, ctx, apiBase, ownerToken, orgID, fastSession.Session.ID)
	slowEvents := agentSessionsListAllEvents(t, ctx, apiBase, ownerToken, orgID, slowSession.Session.ID)
	assertAgentSessionsHistoryMatchesLiveMarkers(t, fastEvents, fastFinalMarker)
	assertAgentSessionsHistoryMatchesLiveMarkers(t, slowEvents, slowFinalMarker)
}

func assertAgentSessionsSameSandboxAccess(t *testing.T, access agentSessionsSandboxAccess, sessionID, sandboxID, sandboxBaseURL string) {
	t.Helper()
	requireAgentSessionsSandboxStreamAccess(t, access, sessionID)
	if access.SandboxID != sandboxID {
		t.Fatalf("session %s sandbox_id=%s want shared sandbox %s", sessionID, access.SandboxID, sandboxID)
	}
	if access.SandboxBaseURL != sandboxBaseURL {
		t.Fatalf("session %s sandbox_base_url=%q want shared sandbox %q", sessionID, access.SandboxBaseURL, sandboxBaseURL)
	}
}
