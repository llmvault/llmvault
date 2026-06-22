package e2e

import (
	"context"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sandbox"
)

type agentSessionsSandboxUpgrade struct {
	UpgradeID    string  `json:"upgrade_id"`
	Status       string  `json:"status"`
	Phase        string  `json:"phase"`
	OldSandboxID *string `json:"old_sandbox_id,omitempty"`
	NewSandboxID *string `json:"new_sandbox_id,omitempty"`
	ErrorMessage *string `json:"error_message,omitempty"`
}

func TestAgentSessionsSandboxUpgradeDrainE2E(t *testing.T) {
	if os.Getenv("HIVY_AGENT_SESSIONS_E2E") != "1" {
		t.Skip("set HIVY_AGENT_SESSIONS_E2E=1 to run against the live compose stack")
	}
	loadEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	apiBase := agentSessionsBaseURL("HIVY_API_BASE_URL", "HIVY_COMPOSE_API_PORT", "8080")
	workerBase := agentSessionsBaseURL("HIVY_WORKER_BASE_URL", "HIVY_COMPOSE_WORKER_HEALTH_PORT", "8090")
	requireAgentSessionsHealthy(t, ctx, apiBase, "api")
	requireAgentSessionsHealthy(t, ctx, workerBase, "worker")
	agentSessionsEnsureSystemOpenRouterCredential(t)

	runID := strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	password := "agent-sessions-drain-password"
	ownerEmail := "agent-sessions-drain-" + runID + "@example.com"
	finalMarker := "AGENT_SESSIONS_DRAIN_E2E_PASS_" + runID
	stepMarkerPrefix := "AGENT_SESSIONS_DRAIN_E2E_STEP_" + runID + "_"

	t.Logf("registering drain e2e owner=%s", ownerEmail)
	ownerAuth := agentSessionsRegister(t, ctx, apiBase, ownerEmail, password, "Agent Sessions Drain "+runID)
	if len(ownerAuth.Orgs) != 1 || ownerAuth.Orgs[0].ID == "" {
		t.Fatalf("owner register did not return one org: %+v", ownerAuth.Orgs)
	}
	orgID := ownerAuth.Orgs[0].ID
	ownerToken := ownerAuth.AccessToken

	agents := agentSessionsListAgents(t, ctx, apiBase, ownerToken, orgID)
	agent := findDefaultAgent(t, agents)
	assertAgentSessionsAgentSandboxImage(t, "default Hivy drain", agent, model.SandboxImageDefault)
	channels := agentSessionsListChannels(t, ctx, apiBase, ownerToken, orgID)
	general := findDefaultGeneralChannel(t, channels, agent.ID)

	session := agentSessionsCreateSessionWithPayload(t, ctx, apiBase, ownerToken, orgID, map[string]any{
		"channel_id":       general.ID,
		"reasoning_effort": "low",
		"text":             agentSessionsDrainPrompt(stepMarkerPrefix, finalMarker),
		"access_mode":      "full",
	})
	if session.Session.ID == "" {
		t.Fatalf("drain session was not created correctly: %+v", session)
	}
	assertAgentSessionsBackendOwnedMutationEvent(t, session.Event)
	t.Logf("created drain session id=%s queued=%t", session.Session.ID, session.Queued)

	stream := agentSessionsStartSandboxStream(t, ctx, apiBase, ownerToken, orgID, session.Session.ID)
	firstTool := stream.waitForEvent(t, ctx, 2*time.Minute, func(event runtimeSSEEvent) bool {
		return event.Name == "tool_call" && strings.Contains(event.RawData, stepMarkerPrefix+"1")
	})
	t.Logf("drain test first bash tool call observed event=%s", firstTool.RawData)

	oldSandbox := agentSessionsWaitForSessionSandbox(t, ctx, orgID, session.Session.ID)
	t.Logf("old sandbox before drain id=%s status=%s external_id=%s", oldSandbox.ID, oldSandbox.Status, oldSandbox.ExternalID)
	t.Cleanup(func() {
		agentSessionsDeleteSandbox(t, ctx, apiBase, ownerToken, orgID, oldSandbox.ID.String())
	})

	upgradeStartedAt := time.Now()
	upgrade := agentSessionsStartSandboxUpgrade(t, ctx, apiBase, ownerToken, orgID, agent.ID)
	if upgrade.OldSandboxID == nil || *upgrade.OldSandboxID != oldSandbox.ID.String() {
		t.Fatalf("upgrade old_sandbox_id=%v want %s", upgrade.OldSandboxID, oldSandbox.ID)
	}
	t.Logf("started sandbox upgrade id=%s phase=%s status=%s", upgrade.UpgradeID, upgrade.Phase, upgrade.Status)

	drainingPhase := agentSessionsWaitForSandboxUpgradePhase(t, ctx, apiBase, ownerToken, orgID, agent.ID, upgrade.UpgradeID, model.AgentSandboxUpgradePhaseDrainingOld, 8*time.Minute)
	if drainingPhase.NewSandboxID == nil || *drainingPhase.NewSandboxID == "" {
		t.Fatalf("draining upgrade has no new_sandbox_id: %+v", drainingPhase)
	}
	drainStartedAt := time.Now()
	oldDraining := agentSessionsWaitForSandboxDBStatus(t, ctx, oldSandbox.ID, string(sandbox.StatusDraining))
	t.Logf("old sandbox entered drain id=%s status=%s new_sandbox=%s", oldDraining.ID, oldDraining.Status, *drainingPhase.NewSandboxID)

	stillDraining := agentSessionsGetSandboxUpgrade(t, ctx, apiBase, ownerToken, orgID, agent.ID, upgrade.UpgradeID)
	if stillDraining.Status == model.AgentSandboxUpgradeStatusSucceeded {
		t.Fatalf("upgrade succeeded while old sandbox had an active turn: %+v", stillDraining)
	}

	agentSessionsSendMessageStatus(t, ctx, apiBase, ownerToken, orgID, session.Session.ID, "This message should be rejected while the sandbox is draining.", http.StatusConflict)
	agentSessionsCreateSessionWithPayloadStatus(t, ctx, apiBase, ownerToken, orgID, map[string]any{
		"channel_id": general.ID,
		"text":       "This new session should be rejected while the agent sandbox is draining.",
	}, http.StatusConflict)

	finalEvent := stream.waitForEvent(t, ctx, 6*time.Minute, func(event runtimeSSEEvent) bool {
		return strings.Contains(event.RawData, finalMarker)
	})
	t.Logf("drain session final observed event=%s", finalEvent.RawData)
	finalResponse := waitForAgentSessionsResponse(t, ctx, apiBase, ownerToken, orgID, session.Session.ID, finalMarker)
	t.Logf("drain session final persisted event_id=%s type=%s", finalResponse.ID, finalResponse.EventType)

	completed := agentSessionsWaitForSandboxUpgradeSucceeded(t, ctx, apiBase, ownerToken, orgID, agent.ID, upgrade.UpgradeID, 10*time.Minute)
	if completed.NewSandboxID == nil || *completed.NewSandboxID == "" || *completed.NewSandboxID == oldSandbox.ID.String() {
		t.Fatalf("bad completed upgrade sandbox ids old=%s upgrade=%+v", oldSandbox.ID, completed)
	}
	t.Logf("upgrade completed id=%s total=%s drain_wait=%s new_sandbox=%s", completed.UpgradeID, time.Since(upgradeStartedAt), time.Since(drainStartedAt), *completed.NewSandboxID)

	agents = agentSessionsListAgents(t, ctx, apiBase, ownerToken, orgID)
	agent = findDefaultAgent(t, agents)
	if agent.Sandbox == nil {
		t.Fatalf("default agent has no active sandbox after drain upgrade")
	}
	if agent.Sandbox.ID != *completed.NewSandboxID {
		t.Fatalf("active sandbox id=%s want upgraded sandbox %s agent=%+v", agent.Sandbox.ID, *completed.NewSandboxID, agent)
	}
	if agent.Sandbox.Status != string(sandbox.StatusRunning) {
		t.Fatalf("active sandbox status=%s want running agent=%+v", agent.Sandbox.Status, agent)
	}
	assertAgentSessionsDockerContainerImage(t, ctx, "default Hivy upgraded", agent.Sandbox.ExternalID, defaultAgentSessionsSandboxRuntimeImage())
	t.Cleanup(func() {
		agentSessionsDeleteSandbox(t, ctx, apiBase, ownerToken, orgID, agent.Sandbox.ID)
	})

	events := agentSessionsListAllEvents(t, ctx, apiBase, ownerToken, orgID, session.Session.ID)
	assertPersistedFinals(t, events, finalMarker)
	assertAgentSessionsHasTurnCompleted(t, events)
}

func agentSessionsDrainPrompt(stepMarkerPrefix, finalMarker string) string {
	commands := make([]string, 0, 5)
	for i := 1; i <= 5; i++ {
		commands = append(commands, "python3 -c 'import time; time.sleep(8); print(\""+stepMarkerPrefix+strconv.Itoa(i)+"\")'")
	}
	return strings.Join([]string{
		"This is the agent sessions sandbox drain upgrade E2E.",
		"Before replying, call bash exactly five times. Use one bash tool call per command and do not combine commands.",
		"Run these commands in order:",
		"1. " + commands[0],
		"2. " + commands[1],
		"3. " + commands[2],
		"4. " + commands[3],
		"5. " + commands[4],
		"After the fifth bash result, visible final reply exactly " + finalMarker + " and no other text.",
	}, "\n")
}

func agentSessionsStartSandboxUpgrade(t *testing.T, ctx context.Context, baseURL, token, orgID, agentID string) agentSessionsSandboxUpgrade {
	t.Helper()
	var out agentSessionsSandboxUpgrade
	agentSessionsJSON(t, ctx, http.MethodPost, baseURL+"/v1/agents/"+agentID+"/sandbox/upgrade", token, orgID, nil, http.StatusAccepted, &out)
	if out.UpgradeID == "" {
		t.Fatalf("sandbox upgrade returned empty id: %+v", out)
	}
	return out
}

func agentSessionsGetSandboxUpgrade(t *testing.T, ctx context.Context, baseURL, token, orgID, agentID, upgradeID string) agentSessionsSandboxUpgrade {
	t.Helper()
	var out agentSessionsSandboxUpgrade
	agentSessionsJSON(t, ctx, http.MethodGet, baseURL+"/v1/agents/"+agentID+"/sandbox/upgrades/"+upgradeID, token, orgID, nil, http.StatusOK, &out)
	return out
}

func agentSessionsWaitForSandboxUpgradePhase(t *testing.T, ctx context.Context, baseURL, token, orgID, agentID, upgradeID, phase string, timeout time.Duration) agentSessionsSandboxUpgrade {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last agentSessionsSandboxUpgrade
	for time.Now().Before(deadline) {
		last = agentSessionsGetSandboxUpgrade(t, ctx, baseURL, token, orgID, agentID, upgradeID)
		if last.Phase == phase {
			return last
		}
		if last.Status == model.AgentSandboxUpgradeStatusSucceeded || last.Status == model.AgentSandboxUpgradeStatusFailed {
			t.Fatalf("upgrade reached terminal status before phase %s: %+v", phase, last)
		}
		t.Logf("waiting for upgrade phase=%s last_status=%s last_phase=%s", phase, last.Status, last.Phase)
		select {
		case <-ctx.Done():
			t.Fatalf("context expired waiting for upgrade phase: %v", ctx.Err())
		case <-time.After(2 * time.Second):
		}
	}
	t.Fatalf("timed out waiting for upgrade phase=%s last=%+v", phase, last)
	return agentSessionsSandboxUpgrade{}
}

func agentSessionsWaitForSandboxUpgradeSucceeded(t *testing.T, ctx context.Context, baseURL, token, orgID, agentID, upgradeID string, timeout time.Duration) agentSessionsSandboxUpgrade {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last agentSessionsSandboxUpgrade
	for time.Now().Before(deadline) {
		last = agentSessionsGetSandboxUpgrade(t, ctx, baseURL, token, orgID, agentID, upgradeID)
		if last.Status == model.AgentSandboxUpgradeStatusSucceeded {
			return last
		}
		if last.Status == model.AgentSandboxUpgradeStatusFailed {
			t.Fatalf("upgrade failed: %+v", last)
		}
		t.Logf("waiting for upgrade success status=%s phase=%s", last.Status, last.Phase)
		select {
		case <-ctx.Done():
			t.Fatalf("context expired waiting for upgrade success: %v", ctx.Err())
		case <-time.After(2 * time.Second):
		}
	}
	t.Fatalf("timed out waiting for upgrade success last=%+v", last)
	return agentSessionsSandboxUpgrade{}
}

func assertAgentSessionsHasTurnCompleted(t *testing.T, events []agentSessionsEvent) {
	t.Helper()
	for _, event := range events {
		if event.EventType == "turn_completed" {
			return
		}
	}
	t.Fatalf("session events missing turn_completed; events=%s", summarizeSessionEventTypes(events))
}
