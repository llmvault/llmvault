package e2e

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestAgentSessionsMemoryE2E(t *testing.T) {
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
	agentSessionsEnsureSystemAtlasCloudCredential(t)

	runID := strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	password := "agent-sessions-memory-e2e-password"
	ownerEmail := "agent-sessions-memory-" + runID + "@example.com"
	finalMarker := "MEMORY_E2E_PASS_" + runID
	orgMarker := "ORG_MEMORY_" + runID
	userMarker := "USER_MEMORY_" + runID

	ownerAuth := agentSessionsRegister(t, ctx, apiBase, ownerEmail, password, "Agent Memory Owner "+runID)
	orgID := ownerAuth.Orgs[0].ID
	ownerToken := ownerAuth.AccessToken
	agents := agentSessionsListAgents(t, ctx, apiBase, ownerToken, orgID)
	defaultAgent := findDefaultAgent(t, agents)
	channels := agentSessionsListChannels(t, ctx, apiBase, ownerToken, orgID)
	general := findDefaultGeneralChannel(t, channels, defaultAgent.ID)

	orgMemory := agentSessionsCreateMemory(t, ctx, apiBase, ownerToken, orgID, map[string]any{
		"content": "Organization memory marker: " + orgMarker + ". The launch codename is Helio.",
		"tags":    []string{"e2e", "memory", "launch"},
	})
	userMemory := agentSessionsCreateMemory(t, ctx, apiBase, ownerToken, orgID, map[string]any{
		"channel_id": general.ID,
		"content":    "Channel memory marker: " + userMarker + ". The preferred escalation word is Prism.",
		"tags":       []string{"e2e", "memory", "escalation"},
	})
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), time.Minute)
		defer cleanupCancel()
		agentSessionsArchiveMemory(t, cleanupCtx, apiBase, ownerToken, orgID, orgMemory.ID)
		agentSessionsArchiveMemory(t, cleanupCtx, apiBase, ownerToken, orgID, userMemory.ID)
	})
	waitForAgentSessionsMemoryReady(t, ctx, apiBase, ownerToken, orgID, orgMemory.ID)
	waitForAgentSessionsMemoryReady(t, ctx, apiBase, ownerToken, orgID, userMemory.ID)

	query := url.Values{"q": []string{"launch codename escalation word"}, "limit": []string{"10"}}
	searchHits := agentSessionsListMemories(t, ctx, apiBase, ownerToken, orgID, query)
	assertAgentSessionsMemorySearchHits(t, searchHits, orgMarker, userMarker)

	session := agentSessionsCreateSession(t, ctx, apiBase, ownerToken, orgID, general.ID, strings.Join([]string{
		"This is the memory E2E.",
		"Use preloaded memory context only.",
		"Reply exactly " + finalMarker + " followed by the organization launch codename and the user escalation word, separated by spaces, and no other text.",
	}, "\n"))
	assertAgentSessionsBackendOwnedMutationEvent(t, session.Event)
	stream := agentSessionsStartSandboxStream(t, ctx, apiBase, ownerToken, orgID, session.Session.ID)
	stream.waitForEvent(t, ctx, 3*time.Minute, func(event runtimeSSEEvent) bool {
		return strings.Contains(event.RawData, finalMarker) &&
			strings.Contains(event.RawData, "Helio") &&
			strings.Contains(event.RawData, "Prism")
	})
	waitForAgentSessionsResponse(t, ctx, apiBase, ownerToken, orgID, session.Session.ID, finalMarker)
	events := agentSessionsListAllEvents(t, ctx, apiBase, ownerToken, orgID, session.Session.ID)
	assertAgentSessionsUserEventsDoNotStoreDynamicContext(t, events)
}

func assertAgentSessionsMemorySearchHits(t *testing.T, hits []agentSessionsMemory, markers ...string) {
	t.Helper()
	raw, _ := json.Marshal(hits)
	for _, marker := range markers {
		if !strings.Contains(string(raw), marker) {
			t.Fatalf("memory search did not include marker=%s hits=%s", marker, raw)
		}
	}
}
