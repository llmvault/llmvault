package e2e

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

func TestAgentSessionsSleepWakeCronLifecycleE2E(t *testing.T) {
	loadEnv(t)
	if os.Getenv("HIVY_AGENT_SESSIONS_E2E") != "1" {
		t.Skip("set HIVY_AGENT_SESSIONS_E2E=1 to run against the live compose stack")
	}
	if os.Getenv("HIVY_AGENT_SESSIONS_LIFECYCLE_E2E") != "1" {
		t.Skip("set HIVY_AGENT_SESSIONS_LIFECYCLE_E2E=1 to run the sleep/wake cron lifecycle E2E")
	}
	if os.Getenv("HIVY_SANDBOX_IDLE_TIMEOUT") != "30s" {
		t.Skip("start API and worker with HIVY_SANDBOX_IDLE_TIMEOUT=30s before running this E2E")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 14*time.Minute)
	defer cancel()

	apiBase := agentSessionsBaseURL("HIVY_API_BASE_URL", "HIVY_COMPOSE_API_PORT", "8080")
	workerBase := agentSessionsBaseURL("HIVY_WORKER_BASE_URL", "HIVY_COMPOSE_WORKER_HEALTH_PORT", "8090")
	requireAgentSessionsHealthy(t, ctx, apiBase, "api")
	requireAgentSessionsHealthy(t, ctx, workerBase, "worker")
	agentSessionsEnsureSystemOpenRouterCredential(t)

	runID := strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	password := "agent-sessions-lifecycle-password"
	ownerEmail := "agent-sessions-lifecycle-" + runID + "@example.com"
	ownerAuth := agentSessionsRegister(t, ctx, apiBase, ownerEmail, password, "Agent Sessions Lifecycle "+runID)
	orgID := ownerAuth.Orgs[0].ID
	ownerToken := ownerAuth.AccessToken

	agents := agentSessionsListAgents(t, ctx, apiBase, ownerToken, orgID)
	defaultAgent := findDefaultAgent(t, agents)
	assertAgentSessionsAgentSandboxImage(t, "lifecycle Hivy", defaultAgent, model.SandboxImageDefault)
	channels := agentSessionsListChannels(t, ctx, apiBase, ownerToken, orgID)
	general := findDefaultGeneralChannel(t, channels, defaultAgent.ID)

	runAgentSessionsDefaultAgentCronSleepWakeE2E(t, ctx, apiBase, ownerToken, orgID, defaultAgent.ID, general.ID, runID)
	runAgentSessionsCatalogAgentCronSleepWakeE2E(t, ctx, apiBase, ownerToken, orgID, ownerAuth.User.ID, runID)
}

func runAgentSessionsDefaultAgentCronSleepWakeE2E(t *testing.T, ctx context.Context, apiBase, token, orgID, agentID, channelID, runID string) {
	t.Helper()
	initialMarker := "LIFECYCLE_DEFAULT_INITIAL_" + runID
	scheduledMarker := "LIFECYCLE_DEFAULT_SCHEDULED_" + runID
	session := agentSessionsCreateSessionWithPayload(t, ctx, apiBase, token, orgID, map[string]any{
		"channel_id":       channelID,
		"model_definition": map[string]any{"reasoning_effort": "low"},
		"text": strings.Join([]string{
			"This is the default-agent sleep/wake lifecycle E2E.",
			"Reply exactly " + initialMarker + " and no other text.",
		}, "\n"),
	})
	waitForAgentSessionsResponse(t, ctx, apiBase, token, orgID, session.Session.ID, initialMarker)

	sb := agentSessionsWaitForSessionSandbox(t, ctx, orgID, session.Session.ID)
	t.Cleanup(func() {
		agentSessionsDeleteSandbox(t, ctx, apiBase, token, orgID, sb.ID.String())
	})
	assertAgentSessionsDockerContainer(t, ctx, "default-agent initial", sb)
	assertAgentSessionsDockerContainerImage(t, ctx, "default-agent Hivy", sb.ExternalID, defaultAgentSessionsSandboxRuntimeImage())

	schedule := agentSessionsCreateCronViaMCP(t, ctx, orgID, agentID, session.Session.ID, 50, "default-agent "+runID, "Reply exactly "+scheduledMarker+" and no other text.")
	t.Logf("default-agent lifecycle schedule id=%s job=%s next_run_at=%v sandbox=%s container=%s", schedule.ID, schedule.RuntimeJobID, schedule.NextRunAt, sb.ID, sb.ExternalID)

	agentSessionsWaitForSandboxDBStatus(t, ctx, sb.ID, "stopped")
	agentSessionsWaitForDockerRunning(t, ctx, sb.ExternalID, false)

	run := agentSessionsWaitForScheduleRunSession(t, ctx, schedule.ID)
	if run.SessionID == nil {
		t.Fatal("default-agent schedule run missing session")
	}
	scheduledSandbox := agentSessionsWaitForSessionSandbox(t, ctx, orgID, run.SessionID.String())
	t.Cleanup(func() {
		agentSessionsDeleteSandbox(t, ctx, apiBase, token, orgID, scheduledSandbox.ID.String())
	})
	if scheduledSandbox.ID == sb.ID || scheduledSandbox.ExternalID == sb.ExternalID {
		t.Fatalf("default-agent schedule reused old sandbox/container: old=%s/%s new=%s/%s", sb.ID, sb.ExternalID, scheduledSandbox.ID, scheduledSandbox.ExternalID)
	}
	assertAgentSessionsDockerContainer(t, ctx, "default-agent scheduled", scheduledSandbox)
	assertAgentSessionsDockerContainerImage(t, ctx, "default-agent Hivy scheduled", scheduledSandbox.ExternalID, defaultAgentSessionsSandboxRuntimeImage())
	waitForAgentSessionsResponse(t, ctx, apiBase, token, orgID, run.SessionID.String(), scheduledMarker)

	agentSessionsWaitForSandboxDBStatus(t, ctx, sb.ID, "stopped")
	agentSessionsWaitForDockerRunning(t, ctx, sb.ExternalID, false)
	agentSessionsWaitForSandboxDBStatus(t, ctx, scheduledSandbox.ID, "stopped")
	agentSessionsWaitForDockerRunning(t, ctx, scheduledSandbox.ExternalID, false)
}

func runAgentSessionsCatalogAgentCronSleepWakeE2E(t *testing.T, ctx context.Context, apiBase, token, orgID, ownerUserID, runID string) {
	t.Helper()
	githubPluginID := agentSessionsSeedInstalledPluginFixture(t, orgID, ownerUserID, "github")
	t.Logf("seeded GitHub plugin install for session sandbox lifecycle agent plugin_id=%s", githubPluginID)

	agent := agentSessionsInstallCatalogAgent(t, ctx, apiBase, token, orgID, "hakaree")
	assertAgentSessionsAgentSandboxImage(t, "lifecycle Hakaree", agent, model.SandboxImageDeveloper)
	channel := agentSessionsCreateChannel(t, ctx, apiBase, token, orgID, "lifecycle-hakaree-"+runID, agent.ID)

	initialMarker := "LIFECYCLE_SESSION_INITIAL_" + runID
	scheduledMarker := "LIFECYCLE_SESSION_SCHEDULED_" + runID
	session := agentSessionsCreateSessionWithPayload(t, ctx, apiBase, token, orgID, map[string]any{
		"channel_id":       channel.ID,
		"model_definition": map[string]any{"reasoning_effort": "low"},
		"text": strings.Join([]string{
			"This is the session sandbox sleep/wake lifecycle E2E.",
			"Reply exactly " + initialMarker + " and no other text.",
		}, "\n"),
	})
	waitForAgentSessionsResponse(t, ctx, apiBase, token, orgID, session.Session.ID, initialMarker)

	oldSandbox := agentSessionsWaitForSessionSandbox(t, ctx, orgID, session.Session.ID)
	t.Cleanup(func() {
		agentSessionsDeleteSandbox(t, ctx, apiBase, token, orgID, oldSandbox.ID.String())
	})
	assertAgentSessionsDockerContainer(t, ctx, "session sandbox initial", oldSandbox)
	expectedDeveloperImage := developerAgentSessionsSandboxRuntimeImage()
	assertAgentSessionsDockerContainerImage(t, ctx, "session sandbox Hakaree initial", oldSandbox.ExternalID, expectedDeveloperImage)

	schedule := agentSessionsCreateCronViaMCP(t, ctx, orgID, agent.ID, session.Session.ID, 50, "session sandbox "+runID, "Reply exactly "+scheduledMarker+" and no other text.")
	t.Logf("session sandbox lifecycle schedule id=%s job=%s next_run_at=%v old_sandbox=%s old_container=%s", schedule.ID, schedule.RuntimeJobID, schedule.NextRunAt, oldSandbox.ID, oldSandbox.ExternalID)

	agentSessionsWaitForSandboxDBStatus(t, ctx, oldSandbox.ID, "stopped")
	agentSessionsWaitForDockerRunning(t, ctx, oldSandbox.ExternalID, false)

	run := agentSessionsWaitForScheduleRunSession(t, ctx, schedule.ID)
	if run.SessionID == nil {
		t.Fatal("session sandbox schedule run missing session")
	}
	scheduledSandbox := agentSessionsWaitForSessionSandbox(t, ctx, orgID, run.SessionID.String())
	t.Cleanup(func() {
		agentSessionsDeleteSandbox(t, ctx, apiBase, token, orgID, scheduledSandbox.ID.String())
	})
	if scheduledSandbox.ID == oldSandbox.ID || scheduledSandbox.ExternalID == oldSandbox.ExternalID {
		t.Fatalf("session sandbox schedule reused old sandbox/container: old=%s/%s new=%s/%s", oldSandbox.ID, oldSandbox.ExternalID, scheduledSandbox.ID, scheduledSandbox.ExternalID)
	}
	assertAgentSessionsDockerContainer(t, ctx, "session sandbox scheduled", scheduledSandbox)
	assertAgentSessionsDockerContainerImage(t, ctx, "session sandbox Hakaree scheduled", scheduledSandbox.ExternalID, expectedDeveloperImage)
	waitForAgentSessionsResponse(t, ctx, apiBase, token, orgID, run.SessionID.String(), scheduledMarker)

	agentSessionsWaitForSandboxDBStatus(t, ctx, oldSandbox.ID, "stopped")
	agentSessionsWaitForDockerRunning(t, ctx, oldSandbox.ExternalID, false)
	agentSessionsWaitForSandboxDBStatus(t, ctx, scheduledSandbox.ID, "stopped")
	agentSessionsWaitForDockerRunning(t, ctx, scheduledSandbox.ExternalID, false)
}
