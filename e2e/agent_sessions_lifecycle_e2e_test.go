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

	runAgentSessionsAlwaysOnCronSleepWakeE2E(t, ctx, apiBase, ownerToken, orgID, defaultAgent.ID, general.ID, runID)
	runAgentSessionsPerSessionCronSleepWakeE2E(t, ctx, apiBase, ownerToken, orgID, ownerAuth.User.ID, runID)
}

func runAgentSessionsAlwaysOnCronSleepWakeE2E(t *testing.T, ctx context.Context, apiBase, token, orgID, agentID, channelID, runID string) {
	t.Helper()
	initialMarker := "LIFECYCLE_ALWAYS_INITIAL_" + runID
	scheduledMarker := "LIFECYCLE_ALWAYS_SCHEDULED_" + runID
	session := agentSessionsCreateSessionWithPayload(t, ctx, apiBase, token, orgID, map[string]any{
		"channel_id":       channelID,
		"model_definition": map[string]any{"reasoning_effort": "low"},
		"text": strings.Join([]string{
			"This is the always-on sleep/wake lifecycle E2E.",
			"Reply exactly " + initialMarker + " and no other text.",
		}, "\n"),
	})
	waitForAgentSessionsResponse(t, ctx, apiBase, token, orgID, session.Session.ID, initialMarker)

	sb := agentSessionsWaitForSessionSandbox(t, ctx, orgID, session.Session.ID)
	t.Cleanup(func() {
		agentSessionsDeleteSandbox(t, ctx, apiBase, token, orgID, sb.ID.String())
	})
	assertAgentSessionsDockerContainer(t, ctx, "always-on initial", sb)
	assertAgentSessionsDockerContainerImage(t, ctx, "always-on Hivy", sb.ExternalID, defaultAgentSessionsSandboxRuntimeImage())

	schedule := agentSessionsCreateCronViaMCP(t, ctx, orgID, agentID, session.Session.ID, 50, "always-on "+runID, "Reply exactly "+scheduledMarker+" and no other text.")
	t.Logf("always-on lifecycle schedule id=%s job=%s next_run_at=%v sandbox=%s container=%s", schedule.ID, schedule.RuntimeJobID, schedule.NextRunAt, sb.ID, sb.ExternalID)

	agentSessionsWaitForSandboxDBStatus(t, ctx, sb.ID, "stopped")
	agentSessionsWaitForDockerRunning(t, ctx, sb.ExternalID, false)

	run := agentSessionsWaitForScheduleRunSession(t, ctx, schedule.ID)
	if run.SessionID == nil {
		t.Fatal("always-on schedule run missing session")
	}
	waitForAgentSessionsResponse(t, ctx, apiBase, token, orgID, run.SessionID.String(), scheduledMarker)

	woken := agentSessionsWaitForSandboxDBStatus(t, ctx, sb.ID, "running")
	if woken.ExternalID != sb.ExternalID {
		t.Fatalf("always-on schedule woke container %s, want original %s", woken.ExternalID, sb.ExternalID)
	}
	agentSessionsWaitForDockerRunning(t, ctx, sb.ExternalID, true)

	agentSessionsWaitForSandboxDBStatus(t, ctx, sb.ID, "stopped")
	agentSessionsWaitForDockerRunning(t, ctx, sb.ExternalID, false)
}

func runAgentSessionsPerSessionCronSleepWakeE2E(t *testing.T, ctx context.Context, apiBase, token, orgID, ownerUserID, runID string) {
	t.Helper()
	githubPluginID := agentSessionsSeedInstalledPluginFixture(t, orgID, ownerUserID, "github")
	t.Logf("seeded GitHub plugin install for per-session lifecycle agent plugin_id=%s", githubPluginID)

	agent := agentSessionsInstallCatalogAgent(t, ctx, apiBase, token, orgID, "hakaree")
	if agent.SandboxStrategy != "per_session" {
		t.Fatalf("installed Hakaree sandbox_strategy=%q want per_session", agent.SandboxStrategy)
	}
	assertAgentSessionsAgentSandboxImage(t, "lifecycle Hakaree", agent, model.SandboxImageDeveloper)
	channel := agentSessionsCreateChannel(t, ctx, apiBase, token, orgID, "lifecycle-hakaree-"+runID, agent.ID)

	initialMarker := "LIFECYCLE_PER_SESSION_INITIAL_" + runID
	scheduledMarker := "LIFECYCLE_PER_SESSION_SCHEDULED_" + runID
	session := agentSessionsCreateSessionWithPayload(t, ctx, apiBase, token, orgID, map[string]any{
		"channel_id":       channel.ID,
		"model_definition": map[string]any{"reasoning_effort": "low"},
		"text": strings.Join([]string{
			"This is the per-session sleep/wake lifecycle E2E.",
			"Reply exactly " + initialMarker + " and no other text.",
		}, "\n"),
	})
	waitForAgentSessionsResponse(t, ctx, apiBase, token, orgID, session.Session.ID, initialMarker)

	oldSandbox := agentSessionsWaitForSessionSandbox(t, ctx, orgID, session.Session.ID)
	t.Cleanup(func() {
		agentSessionsDeleteSandbox(t, ctx, apiBase, token, orgID, oldSandbox.ID.String())
	})
	assertAgentSessionsDockerContainer(t, ctx, "per-session initial", oldSandbox)
	expectedDeveloperImage := developerAgentSessionsSandboxRuntimeImage()
	assertAgentSessionsDockerContainerImage(t, ctx, "per-session Hakaree initial", oldSandbox.ExternalID, expectedDeveloperImage)

	schedule := agentSessionsCreateCronViaMCP(t, ctx, orgID, agent.ID, session.Session.ID, 50, "per-session "+runID, "Reply exactly "+scheduledMarker+" and no other text.")
	t.Logf("per-session lifecycle schedule id=%s job=%s next_run_at=%v old_sandbox=%s old_container=%s", schedule.ID, schedule.RuntimeJobID, schedule.NextRunAt, oldSandbox.ID, oldSandbox.ExternalID)

	agentSessionsWaitForSandboxDBStatus(t, ctx, oldSandbox.ID, "stopped")
	agentSessionsWaitForDockerRunning(t, ctx, oldSandbox.ExternalID, false)

	run := agentSessionsWaitForScheduleRunSession(t, ctx, schedule.ID)
	if run.SessionID == nil {
		t.Fatal("per-session schedule run missing session")
	}
	scheduledSandbox := agentSessionsWaitForSessionSandbox(t, ctx, orgID, run.SessionID.String())
	t.Cleanup(func() {
		agentSessionsDeleteSandbox(t, ctx, apiBase, token, orgID, scheduledSandbox.ID.String())
	})
	if scheduledSandbox.ID == oldSandbox.ID || scheduledSandbox.ExternalID == oldSandbox.ExternalID {
		t.Fatalf("per-session schedule reused old sandbox/container: old=%s/%s new=%s/%s", oldSandbox.ID, oldSandbox.ExternalID, scheduledSandbox.ID, scheduledSandbox.ExternalID)
	}
	assertAgentSessionsDockerContainer(t, ctx, "per-session scheduled", scheduledSandbox)
	assertAgentSessionsDockerContainerImage(t, ctx, "per-session Hakaree scheduled", scheduledSandbox.ExternalID, expectedDeveloperImage)
	waitForAgentSessionsResponse(t, ctx, apiBase, token, orgID, run.SessionID.String(), scheduledMarker)

	agentSessionsWaitForSandboxDBStatus(t, ctx, oldSandbox.ID, "stopped")
	agentSessionsWaitForDockerRunning(t, ctx, oldSandbox.ExternalID, false)
	agentSessionsWaitForSandboxDBStatus(t, ctx, scheduledSandbox.ID, "stopped")
	agentSessionsWaitForDockerRunning(t, ctx, scheduledSandbox.ExternalID, false)
}
