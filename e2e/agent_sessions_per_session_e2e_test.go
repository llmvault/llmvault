package e2e

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	dockerclient "github.com/docker/docker/client"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

func runAgentSessionsPerSessionCatalogAgentE2E(t *testing.T, ctx context.Context, apiBase, ownerToken, orgID, ownerUserID, runID string) {
	t.Helper()

	githubPluginID := agentSessionsSeedInstalledPluginFixture(t, orgID, ownerUserID, "github")
	t.Logf("seeded required GitHub plugin install fixture plugin_id=%s", githubPluginID)

	agent := agentSessionsInstallCatalogAgent(t, ctx, apiBase, ownerToken, orgID, "hakaree")
	if agent.SandboxStrategy != "per_session" {
		t.Fatalf("installed Hakaree sandbox_strategy=%q want per_session", agent.SandboxStrategy)
	}
	if agent.Model != "deepseek-v4-flash" {
		t.Fatalf("installed Hakaree model=%q want deepseek-v4-flash", agent.Model)
	}
	assertAgentSessionsAgentSandboxImage(t, "Hakaree", agent, model.SandboxImageDeveloper)
	if agent.Sandbox != nil {
		t.Fatalf("per-session catalog agent should not return an always-on sandbox at install time: %+v", agent.Sandbox)
	}
	t.Logf("installed per-session catalog agent id=%s name=%s", agent.ID, agent.Name)

	channel := agentSessionsCreateChannel(t, ctx, apiBase, ownerToken, orgID, "hakaree-e2e-"+runID, agent.ID)
	if channel.DefaultAgentID != agent.ID {
		t.Fatalf("per-session channel default_agent_id=%s want %s", channel.DefaultAgentID, agent.ID)
	}
	t.Logf("created per-session channel id=%s agent=%s", channel.ID, channel.DefaultAgentID)

	pathA := "/tmp/hivy_per_session_" + runID + "_a.txt"
	pathB := "/tmp/hivy_per_session_" + runID + "_b.txt"
	firstMarkerA := "PER_SESSION_E2E_FIRST_A_" + runID
	firstMarkerB := "PER_SESSION_E2E_FIRST_B_" + runID
	secondMarkerA := "PER_SESSION_E2E_SECOND_A_" + runID
	secondMarkerB := "PER_SESSION_E2E_SECOND_B_" + runID
	stateMarkerA := "PER_SESSION_E2E_STATE_A_" + runID
	stateMarkerB := "PER_SESSION_E2E_STATE_B_" + runID
	missingMarkerA := "PER_SESSION_E2E_MISSING_A_" + runID
	missingMarkerB := "PER_SESSION_E2E_MISSING_B_" + runID

	sessionA := agentSessionsCreateSessionWithPayload(t, ctx, apiBase, ownerToken, orgID, map[string]any{
		"channel_id":       channel.ID,
		"reasoning_effort": "low",
		"text": agentSessionsPerSessionFirstPrompt(
			"session A", pathA, firstMarkerA,
			"python3 -c 'import pathlib,time; time.sleep(10); pathlib.Path(\""+pathA+"\").write_text(\"session-a\"); print(\""+stateMarkerA+"\")'",
		),
	})
	sessionB := agentSessionsCreateSessionWithPayload(t, ctx, apiBase, ownerToken, orgID, map[string]any{
		"channel_id":       channel.ID,
		"reasoning_effort": "low",
		"text": agentSessionsPerSessionFirstPrompt(
			"session B", pathB, firstMarkerB,
			"python3 -c 'import pathlib,time; time.sleep(10); pathlib.Path(\""+pathB+"\").write_text(\"session-b\"); print(\""+stateMarkerB+"\")'",
		),
	})
	if sessionA.Session.AgentID != agent.ID || sessionB.Session.AgentID != agent.ID {
		t.Fatalf("per-session sessions used wrong agent: a=%s b=%s want=%s", sessionA.Session.AgentID, sessionB.Session.AgentID, agent.ID)
	}
	if sessionA.Queued || sessionB.Queued {
		t.Fatalf("per-session first turns should be accepted synchronously: a=%t b=%t", sessionA.Queued, sessionB.Queued)
	}
	if sessionA.Session.SandboxID == nil || sessionB.Session.SandboxID == nil {
		t.Fatalf("per-session sessions should return sandbox ids immediately: a=%v b=%v", sessionA.Session.SandboxID, sessionB.Session.SandboxID)
	}
	t.Logf("created per-session sessions a=%s b=%s", sessionA.Session.ID, sessionB.Session.ID)

	accessA := fetchAgentSessionsSandboxAccess(t, ctx, apiBase, ownerToken, orgID, sessionA.Session.ID)
	accessB := fetchAgentSessionsSandboxAccess(t, ctx, apiBase, ownerToken, orgID, sessionB.Session.ID)
	requireAgentSessionsSandboxStreamAccess(t, accessA, sessionA.Session.ID)
	requireAgentSessionsSandboxStreamAccess(t, accessB, sessionB.Session.ID)
	if accessA.SandboxBaseURL == accessB.SandboxBaseURL {
		t.Fatalf("per-session sandbox base URLs are identical: %s", accessA.SandboxBaseURL)
	}

	streamA := agentSessionsStartSandboxStreamWithAccess(t, ctx, sessionA.Session.ID, accessA)
	streamB := agentSessionsStartSandboxStreamWithAccess(t, ctx, sessionB.Session.ID, accessB)

	sandboxA := agentSessionsWaitForSessionSandbox(t, ctx, orgID, sessionA.Session.ID)
	sandboxB := agentSessionsWaitForSessionSandbox(t, ctx, orgID, sessionB.Session.ID)
	assertAgentSessionsDistinctDockerSandboxes(t, ctx, sandboxA, sandboxB)
	expectedDeveloperImage := developerAgentSessionsSandboxRuntimeImage()
	assertAgentSessionsDockerContainerImage(t, ctx, "Hakaree session A", sandboxA.ExternalID, expectedDeveloperImage)
	assertAgentSessionsDockerContainerImage(t, ctx, "Hakaree session B", sandboxB.ExternalID, expectedDeveloperImage)
	t.Cleanup(func() {
		agentSessionsDeleteSandbox(t, ctx, apiBase, ownerToken, orgID, sandboxA.ID.String())
		agentSessionsDeleteSandbox(t, ctx, apiBase, ownerToken, orgID, sandboxB.ID.String())
	})

	detailA := agentSessionsGetSession(t, ctx, apiBase, ownerToken, orgID, sessionA.Session.ID)
	detailB := agentSessionsGetSession(t, ctx, apiBase, ownerToken, orgID, sessionB.Session.ID)
	if detailA.Session.SandboxID == nil || *detailA.Session.SandboxID != sandboxA.ID.String() {
		t.Fatalf("session A sandbox_id=%v want %s", detailA.Session.SandboxID, sandboxA.ID)
	}
	if detailB.Session.SandboxID == nil || *detailB.Session.SandboxID != sandboxB.ID.String() {
		t.Fatalf("session B sandbox_id=%v want %s", detailB.Session.SandboxID, sandboxB.ID)
	}

	firstA := streamA.waitForEvent(t, ctx, 4*time.Minute, func(event runtimeSSEEvent) bool {
		return strings.Contains(event.RawData, firstMarkerA)
	})
	firstB := streamB.waitForEvent(t, ctx, 4*time.Minute, func(event runtimeSSEEvent) bool {
		return strings.Contains(event.RawData, firstMarkerB)
	})
	t.Logf("per-session first finals observed a=%s b=%s", firstA.RawData, firstB.RawData)
	waitForAgentSessionsResponse(t, ctx, apiBase, ownerToken, orgID, sessionA.Session.ID, firstMarkerA)
	waitForAgentSessionsResponse(t, ctx, apiBase, ownerToken, orgID, sessionB.Session.ID, firstMarkerB)

	msgA := agentSessionsSendMessage(t, ctx, apiBase, ownerToken, orgID, sessionA.Session.ID, agentSessionsPerSessionSecondPrompt(pathA, stateMarkerA, missingMarkerA, secondMarkerA))
	msgB := agentSessionsSendMessage(t, ctx, apiBase, ownerToken, orgID, sessionB.Session.ID, agentSessionsPerSessionSecondPrompt(pathB, stateMarkerB, missingMarkerB, secondMarkerB))
	assertAgentSessionsBackendOwnedMutationEvent(t, msgA.Event)
	assertAgentSessionsBackendOwnedMutationEvent(t, msgB.Event)

	stateA := streamA.waitForEvent(t, ctx, 3*time.Minute, func(event runtimeSSEEvent) bool {
		return strings.Contains(agentSessionsToolResultOutput(event), stateMarkerA)
	})
	stateB := streamB.waitForEvent(t, ctx, 3*time.Minute, func(event runtimeSSEEvent) bool {
		return strings.Contains(agentSessionsToolResultOutput(event), stateMarkerB)
	})
	t.Logf("per-session persisted state observed a=%s b=%s", stateA.RawData, stateB.RawData)

	secondA := streamA.waitForEvent(t, ctx, 3*time.Minute, func(event runtimeSSEEvent) bool {
		return strings.Contains(event.RawData, secondMarkerA)
	})
	secondB := streamB.waitForEvent(t, ctx, 3*time.Minute, func(event runtimeSSEEvent) bool {
		return strings.Contains(event.RawData, secondMarkerB)
	})
	t.Logf("per-session second finals observed a=%s b=%s", secondA.RawData, secondB.RawData)
	waitForAgentSessionsResponse(t, ctx, apiBase, ownerToken, orgID, sessionA.Session.ID, secondMarkerA)
	waitForAgentSessionsResponse(t, ctx, apiBase, ownerToken, orgID, sessionB.Session.ID, secondMarkerB)
}

func agentSessionsPerSessionFirstPrompt(label, path, finalMarker, command string) string {
	return strings.Join([]string{
		"This is the agent sessions per-session sandbox E2E for " + label + ".",
		"Before replying, call bash exactly once with this command: " + command + ".",
		"After the bash result, visible final reply exactly " + finalMarker + " and no other text.",
		"Use this file path only for this session: " + path + ".",
	}, "\n")
}

func agentSessionsPerSessionSecondPrompt(path, presentMarker, missingMarker, finalMarker string) string {
	return strings.Join([]string{
		"Before replying, call bash exactly once with this command: if test -f " + path + "; then echo " + presentMarker + "; else echo " + missingMarker + "; fi.",
		"After the bash result, visible final reply exactly " + finalMarker + " and no other text.",
	}, "\n")
}

func agentSessionsSeedInstalledPluginFixture(t *testing.T, orgIDRaw, userIDRaw, slug string) uuid.UUID {
	t.Helper()
	db := agentSessionsOpenDB(t)
	orgID := uuid.MustParse(orgIDRaw)
	userID := uuid.MustParse(userIDRaw)

	var plugin model.Plugin
	if err := db.Where("slug = ? AND status = ?", slug, model.PluginStatusActive).First(&plugin).Error; err != nil {
		t.Fatalf("load required plugin %q: %v", slug, err)
	}

	var existing model.OrgPluginInstall
	err := db.Where("org_id = ? AND plugin_id = ? AND revoked_at IS NULL", orgID, plugin.ID).First(&existing).Error
	if err == nil {
		return plugin.ID
	}
	if err != gorm.ErrRecordNotFound {
		t.Fatalf("load org plugin install fixture: %v", err)
	}

	install := model.OrgPluginInstall{
		ID:              uuid.New(),
		OrgID:           orgID,
		PluginID:        plugin.ID,
		CreatedByUserID: &userID,
	}
	if err := db.Create(&install).Error; err != nil {
		t.Fatalf("create org plugin install fixture: %v", err)
	}
	t.Cleanup(func() {
		db.Where("org_id = ? AND plugin_id = ?", orgID, plugin.ID).Delete(&model.AgentPluginInstall{})
		db.Where("id = ?", install.ID).Delete(&model.OrgPluginInstall{})
	})
	return plugin.ID
}

func agentSessionsWaitForSessionSandbox(t *testing.T, ctx context.Context, orgIDRaw, sessionIDRaw string) model.Sandbox {
	t.Helper()
	db := agentSessionsOpenDB(t)
	orgID := uuid.MustParse(orgIDRaw)
	sessionID := uuid.MustParse(sessionIDRaw)
	deadline := time.Now().Add(4 * time.Minute)
	last := "not found"
	for time.Now().Before(deadline) {
		var sb model.Sandbox
		err := db.WithContext(ctx).
			Joins("JOIN sessions ON sessions.sandbox_id = sandboxes.id").
			Where("sessions.id = ? AND sessions.org_id = ?", sessionID, orgID).
			Take(&sb).Error
		if err == nil {
			last = fmt.Sprintf("sandbox_id=%s status=%s provider=%s external_id=%s", sb.ID, sb.Status, sb.ProviderID, sb.ExternalID)
			if sb.Status == "running" && sb.ExternalID != "" {
				return sb
			}
		} else if err != gorm.ErrRecordNotFound {
			t.Fatalf("load session sandbox: %v", err)
		}
		t.Logf("waiting for session sandbox session=%s last=%s", sessionID, last)
		select {
		case <-ctx.Done():
			t.Fatalf("context expired waiting for session sandbox: %v", ctx.Err())
		case <-time.After(2 * time.Second):
		}
	}
	t.Fatalf("timed out waiting for session sandbox session=%s last=%s", sessionID, last)
	return model.Sandbox{}
}

func assertAgentSessionsDistinctDockerSandboxes(t *testing.T, ctx context.Context, a, b model.Sandbox) {
	t.Helper()
	if a.ID == b.ID {
		t.Fatalf("per-session sandboxes have the same db id: %s", a.ID)
	}
	if a.ExternalID == b.ExternalID {
		t.Fatalf("per-session sandboxes have the same docker external id: %s", a.ExternalID)
	}
	assertAgentSessionsDockerContainer(t, ctx, "session A", a)
	assertAgentSessionsDockerContainer(t, ctx, "session B", b)
}

func assertAgentSessionsDockerContainer(t *testing.T, ctx context.Context, label string, sb model.Sandbox) {
	t.Helper()
	if sb.ProviderID != "docker" {
		t.Fatalf("%s sandbox provider=%q want docker", label, sb.ProviderID)
	}
	if strings.TrimSpace(sb.ExternalID) == "" {
		t.Fatalf("%s sandbox external_id is empty", label)
	}

	inspectCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		t.Fatalf("%s create docker client: %v", label, err)
	}
	defer cli.Close()
	info, err := cli.ContainerInspect(inspectCtx, sb.ExternalID)
	if err != nil {
		t.Fatalf("%s docker inspect failed for sandbox %s external_id=%s: %v", label, sb.ID, sb.ExternalID, err)
	}
	trimmedName := strings.TrimPrefix(info.Name, "/")
	if info.ID != sb.ExternalID && trimmedName != sb.ExternalID {
		t.Fatalf("%s docker inspect id=%s name=%s want external_id=%s", label, info.ID, trimmedName, sb.ExternalID)
	}
	if info.State == nil || !info.State.Running {
		status := ""
		if info.State != nil {
			status = info.State.Status
		}
		t.Fatalf("%s docker container status=%s want running", label, status)
	}
	t.Logf("%s sandbox id=%s external_id=%s docker_status=%s", label, sb.ID, sb.ExternalID, info.State.Status)
}
