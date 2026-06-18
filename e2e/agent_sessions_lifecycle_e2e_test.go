package e2e

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	dockerclient "github.com/docker/docker/client"
	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/mcpserver"
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
		"reasoning_effort": "low",
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
	channel := agentSessionsCreateChannel(t, ctx, apiBase, token, orgID, "lifecycle-hakaree-"+runID, agent.ID)

	initialMarker := "LIFECYCLE_PER_SESSION_INITIAL_" + runID
	scheduledMarker := "LIFECYCLE_PER_SESSION_SCHEDULED_" + runID
	session := agentSessionsCreateSessionWithPayload(t, ctx, apiBase, token, orgID, map[string]any{
		"channel_id":       channel.ID,
		"reasoning_effort": "low",
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
	waitForAgentSessionsResponse(t, ctx, apiBase, token, orgID, run.SessionID.String(), scheduledMarker)

	agentSessionsWaitForSandboxDBStatus(t, ctx, oldSandbox.ID, "stopped")
	agentSessionsWaitForDockerRunning(t, ctx, oldSandbox.ExternalID, false)
	agentSessionsWaitForSandboxDBStatus(t, ctx, scheduledSandbox.ID, "stopped")
	agentSessionsWaitForDockerRunning(t, ctx, scheduledSandbox.ExternalID, false)
}

func agentSessionsCreateCronViaMCP(t *testing.T, ctx context.Context, orgIDRaw, agentIDRaw, sessionID string, intervalSeconds int64, description, taskPrompt string) model.AgentSchedule {
	t.Helper()
	db := agentSessionsOpenDB(t)
	orgID := uuid.MustParse(orgIDRaw)
	agentID := uuid.MustParse(agentIDRaw)
	jobID := "lifecycle-" + strings.ReplaceAll(uuid.NewString(), "-", "")
	token := &model.Token{
		OrgID: orgID,
		Meta: model.JSON{
			model.TokenMetaType:    model.TokenTypeAgentProxy,
			model.TokenMetaAgentID: agentID.String(),
		},
	}
	server, err := mcpserver.BuildServer(ctx, token, db, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("build Hivy MCP server: %v", err)
	}
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
		t.Fatalf("connect Hivy MCP server: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "agent-sessions-lifecycle-e2e", Version: "v1"}, nil)
	mcpSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect Hivy MCP client: %v", err)
	}
	result, err := mcpSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "cron",
		Arguments: map[string]any{
			"action":             "create",
			"job_id":             jobID,
			"description":        description,
			"task_prompt":        taskPrompt,
			"interval_seconds":   intervalSeconds,
			"repeat_count":       int64(1),
			"_hivy_session_id":   sessionID,
			"cron_expression":    nil,
			"schedule_is_wake":   false,
			"schedule_is_system": false,
		},
	})
	if err != nil {
		t.Fatalf("call Hivy MCP cron create: %v", err)
	}
	if result.IsError {
		t.Fatalf("Hivy MCP cron create failed: %s", agentSessionsMCPText(result))
	}
	var schedule model.AgentSchedule
	if err := db.WithContext(ctx).
		Where("org_id = ? AND agent_id = ? AND runtime_job_id = ?", orgID, agentID, jobID).
		First(&schedule).Error; err != nil {
		t.Fatalf("load schedule created via MCP: %v; result=%s", err, agentSessionsMCPText(result))
	}
	return schedule
}

func agentSessionsMCPText(result *mcp.CallToolResult) string {
	if result == nil {
		return ""
	}
	var parts []string
	for _, item := range result.Content {
		if text, ok := item.(*mcp.TextContent); ok {
			parts = append(parts, text.Text)
		} else {
			raw, _ := json.Marshal(item)
			parts = append(parts, string(raw))
		}
	}
	return strings.Join(parts, "\n")
}

func agentSessionsWaitForScheduleRunSession(t *testing.T, ctx context.Context, scheduleID uuid.UUID) model.AgentScheduleRun {
	t.Helper()
	db := agentSessionsOpenDB(t)
	deadline := time.Now().Add(4 * time.Minute)
	var last string
	for time.Now().Before(deadline) {
		var run model.AgentScheduleRun
		err := db.WithContext(ctx).
			Where("schedule_id = ?", scheduleID).
			Order("created_at ASC").
			Last(&run).Error
		if err == nil {
			last = "status=" + run.Status + " run=" + run.ID.String()
			if run.SessionID != nil {
				return run
			}
		} else if err != gorm.ErrRecordNotFound {
			t.Fatalf("load schedule run: %v", err)
		}
		t.Logf("waiting for schedule run session schedule=%s last=%s", scheduleID, last)
		select {
		case <-ctx.Done():
			t.Fatalf("context expired waiting for schedule run: %v", ctx.Err())
		case <-time.After(2 * time.Second):
		}
	}
	t.Fatalf("timed out waiting for schedule run session schedule=%s last=%s", scheduleID, last)
	return model.AgentScheduleRun{}
}

func agentSessionsWaitForSandboxDBStatus(t *testing.T, ctx context.Context, sandboxID uuid.UUID, status string) model.Sandbox {
	t.Helper()
	db := agentSessionsOpenDB(t)
	deadline := time.Now().Add(3 * time.Minute)
	var last string
	for time.Now().Before(deadline) {
		var sb model.Sandbox
		if err := db.WithContext(ctx).Where("id = ?", sandboxID).First(&sb).Error; err != nil {
			t.Fatalf("load sandbox %s: %v", sandboxID, err)
		}
		last = "status=" + sb.Status + " external_id=" + sb.ExternalID
		if sb.Status == status {
			return sb
		}
		t.Logf("waiting for sandbox status sandbox=%s want=%s last=%s", sandboxID, status, last)
		select {
		case <-ctx.Done():
			t.Fatalf("context expired waiting for sandbox status: %v", ctx.Err())
		case <-time.After(2 * time.Second):
		}
	}
	t.Fatalf("timed out waiting for sandbox status sandbox=%s want=%s last=%s", sandboxID, status, last)
	return model.Sandbox{}
}

func agentSessionsWaitForDockerRunning(t *testing.T, ctx context.Context, externalID string, want bool) {
	t.Helper()
	cli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		t.Fatalf("create docker client: %v", err)
	}
	defer cli.Close()
	deadline := time.Now().Add(2 * time.Minute)
	var last string
	for time.Now().Before(deadline) {
		inspectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		info, err := cli.ContainerInspect(inspectCtx, externalID)
		cancel()
		if err != nil {
			t.Fatalf("inspect docker container %s: %v", externalID, err)
		}
		running := info.State != nil && info.State.Running
		if info.State != nil {
			last = info.State.Status
		}
		if running == want {
			return
		}
		t.Logf("waiting for docker running=%t container=%s last_status=%s", want, externalID, last)
		select {
		case <-ctx.Done():
			t.Fatalf("context expired waiting for docker state: %v", ctx.Err())
		case <-time.After(2 * time.Second):
		}
	}
	t.Fatalf("timed out waiting for docker running=%t container=%s last_status=%s", want, externalID, last)
}
