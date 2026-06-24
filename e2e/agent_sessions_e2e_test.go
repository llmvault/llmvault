package e2e

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

func TestAgentSessionsDefaultGeneralChannelE2E(t *testing.T) {
	if os.Getenv("HIVY_AGENT_SESSIONS_E2E") != "1" {
		t.Skip("set HIVY_AGENT_SESSIONS_E2E=1 to run against the live compose stack")
	}
	loadEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	apiBase := agentSessionsBaseURL("HIVY_API_BASE_URL", "HIVY_COMPOSE_API_PORT", "8080")
	workerBase := agentSessionsBaseURL("HIVY_WORKER_BASE_URL", "HIVY_COMPOSE_WORKER_HEALTH_PORT", "8090")
	requireAgentSessionsHealthy(t, ctx, apiBase, "api")
	requireAgentSessionsHealthy(t, ctx, workerBase, "worker")
	agentSessionsEnsureSystemOpenRouterCredential(t)

	runID := strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	password := "agent-sessions-e2e-password"
	ownerEmail := "agent-sessions-owner-" + runID + "@example.com"
	memberEmail := "agent-sessions-member-" + runID + "@example.com"
	firstMarker := "AGENT_SESSIONS_E2E_PASS_" + runID
	secondMarker := "AGENT_SESSIONS_E2E_MULTI_PASS_" + runID
	thinkingMarker := "AGENT_SESSIONS_E2E_THINKING_" + runID
	interruptMarker := "AGENT_SESSIONS_E2E_INTERRUPT_PASS_" + runID
	interruptRecoveryMarker := "AGENT_SESSIONS_E2E_INTERRUPT_RECOVERY_" + runID
	interruptSentinelMissing := "AGENT_SESSIONS_E2E_INTERRUPT_SENTINEL_MISSING_" + runID
	interruptSentinelExists := "AGENT_SESSIONS_E2E_INTERRUPT_SENTINEL_EXISTS_" + runID
	interruptSentinelPath := "/tmp/hivy_interrupt_" + runID

	t.Logf("registering owner=%s", ownerEmail)
	ownerAuth := agentSessionsRegister(t, ctx, apiBase, ownerEmail, password, "Agent Sessions Owner "+runID)
	if len(ownerAuth.Orgs) != 1 || ownerAuth.Orgs[0].ID == "" {
		t.Fatalf("owner register did not return one org: %+v", ownerAuth.Orgs)
	}
	orgID := ownerAuth.Orgs[0].ID
	ownerToken := ownerAuth.AccessToken

	t.Logf("registering collaborator=%s", memberEmail)
	memberAuth := agentSessionsRegister(t, ctx, apiBase, memberEmail, password, "Agent Sessions Member "+runID)
	agentSessionsAddOrgMemberFixture(t, orgID, memberAuth.User.ID, "member")
	memberToken := agentSessionsLogin(t, ctx, apiBase, memberEmail, password, orgID).AccessToken

	agents := agentSessionsListAgents(t, ctx, apiBase, ownerToken, orgID)
	defaultAgent := findDefaultAgent(t, agents)
	assertAgentSessionsAgentSandboxImage(t, "default Hivy", defaultAgent, model.SandboxImageDefault)
	t.Logf("default agent id=%s name=%s sandbox_present=%t", defaultAgent.ID, defaultAgent.Name, defaultAgent.Sandbox != nil)
	pluginFixture := agentSessionsSeedPluginFixture(t, orgID, ownerAuth.User.ID, runID)
	t.Logf("seeded plugin slug=%s id=%s connection=%s", pluginFixture.PluginSlug, pluginFixture.PluginID, pluginFixture.ConnectionID)

	channels := agentSessionsListChannels(t, ctx, apiBase, ownerToken, orgID)
	general := findDefaultGeneralChannel(t, channels, defaultAgent.ID)
	t.Logf("default #general channel id=%s origin=%s provider=%s", general.ID, general.Origin, general.ExternalProvider)

	session := agentSessionsCreateSession(t, ctx, apiBase, ownerToken, orgID, general.ID, strings.Join([]string{
		"This is the agent sessions E2E.",
		"Before replying, call bash exactly once with this command: python3 -c 'import time; time.sleep(12); print(\"first-turn-tool-done\")'.",
		"After the bash result, emit a hidden thinking segment exactly like <think>" + thinkingMarker + "</think>, then visible final reply exactly " + firstMarker + " and no other text.",
	}, "\n"))
	t.Logf("created session id=%s queued=%t first_event=%s", session.Session.ID, session.Queued, eventType(session.Event))
	if session.Session.ID == "" {
		t.Fatalf("session was not created correctly: %+v", session)
	}
	assertAgentSessionsBackendOwnedMutationEvent(t, session.Event)

	agentSessionsSendMessageStatus(t, ctx, apiBase, memberToken, orgID, session.Session.ID, "I should be blocked before sharing", http.StatusForbidden)
	agentSessionsSandboxAccessStatus(t, ctx, apiBase, memberToken, orgID, session.Session.ID, http.StatusForbidden)
	if session.Session.AgentTurnID == "" {
		t.Fatalf("session create response missing active agent_turn_id: %+v", session.Session)
	}
	ownerSandboxAccess := fetchAgentSessionsSandboxAccess(t, ctx, apiBase, ownerToken, orgID, session.Session.ID)
	firstStream := agentSessionsStartSandboxStreamFromTurnFollowWithAccess(t, ctx, session.Session.ID, ownerSandboxAccess, session.Session.AgentTurnID)
	t.Logf("opened durable direct sandbox stream from active turn id=%s", session.Session.AgentTurnID)
	firstToolEvent := firstStream.waitForEvent(t, ctx, 2*time.Minute, func(event runtimeSSEEvent) bool {
		return event.Name == "tool_call" && strings.Contains(event.RawData, "bash")
	})
	t.Logf("first turn entered tool execution event=%s", firstToolEvent.RawData)
	pluginInstall := agentSessionsInstallPlugin(t, ctx, apiBase, ownerToken, orgID, pluginFixture.PluginSlug)
	if len(pluginInstall.EnabledAgentIDs) == 0 {
		t.Fatalf("plugin install did not enable any agent: %+v", pluginInstall)
	}
	t.Logf("installed plugin slug=%s enabled_agents=%v while first turn was active", pluginInstall.Slug, pluginInstall.EnabledAgentIDs)

	detail := agentSessionsPutParticipant(t, ctx, apiBase, ownerToken, orgID, session.Session.ID, memberAuth.User.ID)
	assertAgentSessionsParticipant(t, detail, memberAuth.User.ID)

	memberMessage := agentSessionsSendMessage(t, ctx, apiBase, memberToken, orgID, session.Session.ID, strings.Join([]string{
		"I am now shared into this session while the first turn is still running.",
		"Emit a hidden thinking segment exactly like <think>" + thinkingMarker + "_SECOND</think>, then visible final reply exactly " + secondMarker + " and no other text.",
	}, "\n"))
	if !memberMessage.Queued {
		t.Fatalf("collaborator message was not queued after sharing: %+v", memberMessage)
	}
	assertAgentSessionsBackendOwnedMutationEvent(t, memberMessage.Event)
	memberSandboxAccess := fetchAgentSessionsSandboxAccess(t, ctx, apiBase, memberToken, orgID, session.Session.ID)
	requireAgentSessionsSandboxStreamAccess(t, memberSandboxAccess, session.Session.ID)
	if memberSandboxAccess.Token == ownerSandboxAccess.Token {
		t.Fatalf("member sandbox token should be independently minted")
	}
	if memberSandboxAccess.SandboxBaseURL != ownerSandboxAccess.SandboxBaseURL {
		t.Fatalf("member sandbox base_url=%q want stable session sandbox %q", memberSandboxAccess.SandboxBaseURL, ownerSandboxAccess.SandboxBaseURL)
	}

	firstMarkerEvent := firstStream.waitForEvent(t, ctx, 3*time.Minute, func(event runtimeSSEEvent) bool {
		return strings.Contains(event.RawData, firstMarker)
	})
	t.Logf("first turn marker observed on direct sandbox stream event=%s", firstMarkerEvent.RawData)
	firstResponse := waitForAgentSessionsResponse(t, ctx, apiBase, ownerToken, orgID, session.Session.ID, firstMarker)
	t.Logf("first agent response observed event_id=%s type=%s", firstResponse.ID, firstResponse.EventType)
	firstTerminalEvent := firstStream.waitForEvent(t, ctx, time.Minute, func(event runtimeSSEEvent) bool {
		return event.Name == "turn_completed" || event.Name == "done"
	})
	t.Logf("first turn terminal observed on durable direct sandbox stream event=%s", firstTerminalEvent.RawData)
	secondMarkerEvent := firstStream.waitForEvent(t, ctx, 3*time.Minute, func(event runtimeSSEEvent) bool {
		return strings.Contains(event.RawData, secondMarker)
	})
	t.Logf("second turn marker observed on same direct sandbox stream event=%s", secondMarkerEvent.RawData)

	secondResponse := waitForAgentSessionsResponse(t, ctx, apiBase, memberToken, orgID, session.Session.ID, secondMarker)
	t.Logf("collaborator agent response observed event_id=%s type=%s", secondResponse.ID, secondResponse.EventType)
	pluginFixture = waitForPluginServiceDiscoverySession(t, ctx, orgID, pluginFixture)
	t.Logf("plugin service discovery session observed install=%s", pluginFixture.InstallID)
	events := agentSessionsListAllEvents(t, ctx, apiBase, ownerToken, orgID, session.Session.ID)
	assertAgentSessionsEventOrder(t, events)
	assertAgentSessionsBackendOwnedUserMessages(t, events, ownerAuth.User.ID, memberAuth.User.ID)
	assertAgentSessionsCanonicalIngestion(t, events, session.Session.ID, "", thinkingMarker, firstMarker, secondMarker)
	assertAgentSessionsHistoryMatchesLiveMarkers(t, events, firstMarker, secondMarker)

	agents = agentSessionsListAgents(t, ctx, apiBase, ownerToken, orgID)
	defaultAgent = findDefaultAgent(t, agents)
	if defaultAgent.Sandbox == nil {
		t.Fatalf("default agent has no sandbox after delivery")
	}
	if defaultAgent.Sandbox.Status == "" || defaultAgent.Sandbox.ID == "" {
		t.Fatalf("bad sandbox summary after delivery: %+v", defaultAgent.Sandbox)
	}
	assertAgentSessionsDockerContainerImage(t, ctx, "default Hivy", defaultAgent.Sandbox.ExternalID, defaultAgentSessionsSandboxRuntimeImage())
	t.Logf("sandbox id=%s status=%s external_id=%s", defaultAgent.Sandbox.ID, defaultAgent.Sandbox.Status, defaultAgent.Sandbox.ExternalID)

	runAgentSessionsSameSandboxDirectStreamsE2E(t, ctx, apiBase, ownerToken, orgID, general.ID, runID, ownerSandboxAccess.SandboxID, ownerSandboxAccess.SandboxBaseURL)

	interruptCommand := "python3 -c 'import pathlib,time; time.sleep(8); pathlib.Path(\"" + interruptSentinelPath + "\").write_text(\"done\")'"
	interruptSession := agentSessionsCreateSession(t, ctx, apiBase, ownerToken, orgID, general.ID, strings.Join([]string{
		"This is the agent sessions interrupt E2E.",
		"Before replying, call bash exactly once with this foreground command: " + interruptCommand + ".",
		"Do not run it in the background. If the bash result completes, final reply exactly " + interruptMarker + " and no other text.",
	}, "\n"))
	t.Logf("created interrupt session id=%s queued=%t", interruptSession.Session.ID, interruptSession.Queued)
	if interruptSession.Session.ID == "" {
		t.Fatalf("interrupt session was not created correctly: %+v", interruptSession)
	}
	assertAgentSessionsBackendOwnedMutationEvent(t, interruptSession.Event)
	interruptStream := agentSessionsStartSandboxStream(t, ctx, apiBase, ownerToken, orgID, interruptSession.Session.ID)
	interruptToolEvent := interruptStream.waitForEvent(t, ctx, 2*time.Minute, func(event runtimeSSEEvent) bool {
		return event.Name == "tool_call" && strings.Contains(event.RawData, interruptSentinelPath)
	})
	t.Logf("interrupt turn entered foreground bash event=%s", interruptToolEvent.RawData)
	recovery := agentSessionsSendMessage(t, ctx, apiBase, ownerToken, orgID, interruptSession.Session.ID, strings.Join([]string{
		"Before replying, call bash exactly once with this command: sleep 10; if test -f " + interruptSentinelPath + "; then echo " + interruptSentinelExists + "; else echo " + interruptSentinelMissing + "; fi.",
		"After the bash result, visible final reply exactly " + interruptRecoveryMarker + " and no other text.",
	}, "\n"))
	if !recovery.Queued {
		t.Fatalf("post-interrupt recovery message should queue behind active interrupted turn: %+v", recovery)
	}
	t.Logf("queued recovery message before interrupt event=%s", eventType(recovery.Event))
	interrupt := agentSessionsInterruptSession(t, ctx, apiBase, ownerToken, orgID, interruptSession.Session.ID)
	if interrupt.SessionID != interruptSession.Session.ID || !interrupt.Interrupted || interrupt.Status != "interrupted" {
		t.Fatalf("bad interrupt response: %+v", interrupt)
	}
	interruptFinal := interruptStream.waitForEvent(t, ctx, time.Minute, func(event runtimeSSEEvent) bool {
		return event.Name == "final" && strings.Contains(event.RawData, "Interrupted.")
	})
	t.Logf("interrupt final observed event=%s", interruptFinal.RawData)
	interruptFailed := interruptStream.waitForEvent(t, ctx, time.Minute, func(event runtimeSSEEvent) bool {
		interrupted, _ := event.Payload["interrupted"].(bool)
		return event.Name == "turn_failed" && interrupted && strings.Contains(event.RawData, "interrupted by user")
	})
	t.Logf("interrupt terminal observed event=%s", interruptFailed.RawData)
	sentinelEvent := interruptStream.waitForEvent(t, ctx, 2*time.Minute, func(event runtimeSSEEvent) bool {
		return strings.Contains(agentSessionsToolResultOutput(event), interruptSentinelMissing) ||
			strings.Contains(agentSessionsToolResultOutput(event), interruptSentinelExists)
	})
	if strings.Contains(agentSessionsToolResultOutput(sentinelEvent), interruptSentinelExists) {
		t.Fatalf("interrupted foreground bash wrote sentinel file; event=%s", sentinelEvent.RawData)
	}
	t.Logf("interrupted foreground bash sentinel check event=%s", sentinelEvent.RawData)
	recoveryMarkerEvent := interruptStream.waitForEvent(t, ctx, 3*time.Minute, func(event runtimeSSEEvent) bool {
		return strings.Contains(event.RawData, interruptRecoveryMarker)
	})
	t.Logf("post-interrupt recovery marker observed event=%s", recoveryMarkerEvent.RawData)
	recoveryResponse := waitForAgentSessionsResponse(t, ctx, apiBase, ownerToken, orgID, interruptSession.Session.ID, interruptRecoveryMarker)
	t.Logf("post-interrupt session response observed event_id=%s type=%s", recoveryResponse.ID, recoveryResponse.EventType)
	interruptStream.waitForEvent(t, ctx, time.Minute, func(event runtimeSSEEvent) bool {
		return event.Name == "turn_completed" || event.Name == "done"
	})

	runAgentSessionsPerSessionCatalogAgentE2E(t, ctx, apiBase, ownerToken, orgID, ownerAuth.User.ID, runID)

	t.Cleanup(func() {
		agentSessionsDeleteSandbox(t, ctx, apiBase, ownerToken, orgID, defaultAgent.Sandbox.ID)
	})
}
