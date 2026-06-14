package e2e

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestAgentSessionsDefaultGeneralChannelE2E(t *testing.T) {
	if os.Getenv("HIVY_AGENT_SESSIONS_E2E") != "1" {
		t.Skip("set HIVY_AGENT_SESSIONS_E2E=1 to run against the live compose stack")
	}
	loadEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	apiBase := agentSessionsBaseURL("HIVY_API_BASE_URL", "HIVY_COMPOSE_API_PORT", "8080")
	workerBase := agentSessionsBaseURL("HIVY_WORKER_BASE_URL", "HIVY_COMPOSE_WORKER_HEALTH_PORT", "8090")
	requireAgentSessionsHealthy(t, ctx, apiBase, "api")
	requireAgentSessionsHealthy(t, ctx, workerBase, "worker")

	runID := strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	password := "agent-sessions-e2e-password"
	ownerEmail := "agent-sessions-owner-" + runID + "@example.com"
	memberEmail := "agent-sessions-member-" + runID + "@example.com"
	firstMarker := "AGENT_SESSIONS_E2E_PASS_" + runID
	secondMarker := "AGENT_SESSIONS_E2E_MULTI_PASS_" + runID

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
	t.Logf("default agent id=%s name=%s sandbox_present=%t", defaultAgent.ID, defaultAgent.Name, defaultAgent.Sandbox != nil)

	channels := agentSessionsListChannels(t, ctx, apiBase, ownerToken, orgID)
	general := findDefaultGeneralChannel(t, channels, defaultAgent.ID)
	t.Logf("default #general channel id=%s origin=%s provider=%s", general.ID, general.Origin, general.ExternalProvider)

	session := agentSessionsCreateSession(t, ctx, apiBase, ownerToken, orgID, general.ID, strings.Join([]string{
		"This is the agent sessions E2E.",
		"Before replying, call bash exactly once with this command: python3 -c 'import time; time.sleep(12); print(\"first-turn-tool-done\")'.",
		"Reply with exactly " + firstMarker + " and no other text.",
	}, "\n"))
	t.Logf("created session id=%s queued=%t first_event=%s", session.Session.ID, session.Queued, eventType(session.Event))
	if !session.Queued || session.Session.ID == "" || session.Event == nil || session.Event.SequenceNumber != 1 {
		t.Fatalf("session was not queued correctly: %+v", session)
	}

	agentSessionsSendMessageStatus(t, ctx, apiBase, memberToken, orgID, session.Session.ID, "I should be blocked before sharing", http.StatusForbidden)
	firstStreamAccess := waitForAgentSessionsStreamAccess(t, ctx, apiBase, ownerToken, orgID, session.Session.ID, session.Event.ID)
	t.Logf("first direct sandbox stream url=%s stream_id=%s event_id=%s", firstStreamAccess.DirectURL, firstStreamAccess.StreamID, firstStreamAccess.SessionEventID)
	firstStream := agentSessionsStartDirectStream(t, ctx, firstStreamAccess.DirectURL, firstStreamAccess.StreamToken)
	firstToolEvent := firstStream.waitForEvent(t, ctx, 2*time.Minute, func(event runtimeSSEEvent) bool {
		return event.Name == "tool_call" && strings.Contains(event.RawData, "bash")
	})
	t.Logf("first turn entered tool execution event=%s", firstToolEvent.RawData)
	agentSessionsStreamAccessStatus(t, ctx, apiBase, memberToken, orgID, session.Session.ID, session.Event.ID, http.StatusForbidden)

	detail := agentSessionsPutParticipant(t, ctx, apiBase, ownerToken, orgID, session.Session.ID, memberAuth.User.ID)
	assertAgentSessionsParticipant(t, detail, memberAuth.User.ID)

	memberMessage := agentSessionsSendMessage(t, ctx, apiBase, memberToken, orgID, session.Session.ID, strings.Join([]string{
		"I am now shared into this session while the first turn is still running.",
		"Reply with exactly " + secondMarker + " and no other text.",
	}, "\n"))
	if !memberMessage.Queued || memberMessage.Event == nil || memberMessage.Event.SequenceNumber <= session.Event.SequenceNumber {
		t.Fatalf("collaborator message was not queued after sharing: %+v", memberMessage)
	}
	agentSessionsStreamAccessStatus(t, ctx, apiBase, memberToken, orgID, session.Session.ID, memberMessage.Event.ID, http.StatusNotFound)

	firstDirectEvents := firstStream.waitDone(t, ctx)
	assertAgentSessionsDirectStream(t, firstDirectEvents, firstMarker)
	firstResponse := waitForAgentSessionsResponse(t, ctx, apiBase, ownerToken, orgID, session.Session.ID, firstMarker)
	t.Logf("first agent response observed event_id=%s type=%s", firstResponse.ID, firstResponse.EventType)
	stream := waitForAgentSessionsStreamAccess(t, ctx, apiBase, memberToken, orgID, session.Session.ID, memberMessage.Event.ID)
	t.Logf("direct sandbox stream url=%s stream_id=%s event_id=%s", stream.DirectURL, stream.StreamID, stream.SessionEventID)
	directEvents := agentSessionsReadDirectStream(t, ctx, stream.DirectURL, stream.StreamToken)
	assertAgentSessionsDirectStream(t, directEvents, secondMarker)

	secondResponse := waitForAgentSessionsResponse(t, ctx, apiBase, memberToken, orgID, session.Session.ID, secondMarker)
	t.Logf("collaborator agent response observed event_id=%s type=%s", secondResponse.ID, secondResponse.EventType)
	assertAgentSessionsEventOrder(t, agentSessionsListEvents(t, ctx, apiBase, ownerToken, orgID, session.Session.ID))

	agents = agentSessionsListAgents(t, ctx, apiBase, ownerToken, orgID)
	defaultAgent = findDefaultAgent(t, agents)
	if defaultAgent.Sandbox == nil {
		t.Fatalf("default agent has no sandbox after delivery")
	}
	if defaultAgent.Sandbox.Status == "" || defaultAgent.Sandbox.ID == "" {
		t.Fatalf("bad sandbox summary after delivery: %+v", defaultAgent.Sandbox)
	}
	if !looksLikeDockerContainerID(defaultAgent.Sandbox.ExternalID) {
		t.Fatalf("sandbox external_id does not look like a Docker container id: %q", defaultAgent.Sandbox.ExternalID)
	}
	t.Logf("sandbox id=%s status=%s external_id=%s", defaultAgent.Sandbox.ID, defaultAgent.Sandbox.Status, defaultAgent.Sandbox.ExternalID)
	t.Cleanup(func() {
		agentSessionsDeleteSandbox(t, ctx, apiBase, ownerToken, orgID, defaultAgent.Sandbox.ID)
	})
}
