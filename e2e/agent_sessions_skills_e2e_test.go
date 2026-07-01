package e2e

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

// TestAgentSessionsSkillsMaterializeE2E is the flagship end-to-end test for the
// skills-on-MCP architecture. It boots against the live compose stack (real API +
// worker + Docker sandbox driver) and proves the full chain:
//
//	DB skill (installed via plugin) -> Hivy MCP skill_view tool -> runtime
//	materialize hook -> sandbox filesystem -> executable script.
//
// It asserts the agent can (1) discover the skill via skills_list, (2) load it
// via skill_view (which materializes the whole bundle to .skills/<slug>/),
// (3) read a linked reference via skill_view(name, file_path), and (4) execute a
// script that shipped inside the skill bundle -- the definitive proof that
// materialize copied the script onto the real sandbox disk.
func TestAgentSessionsSkillsMaterializeE2E(t *testing.T) {
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
	password := "agent-sessions-e2e-password"
	ownerEmail := "skills-e2e-owner-" + runID + "@example.com"
	finalMarker := "SKILLS_MCP_E2E_PASS_" + runID
	refMarker := "REFERENCE_MARKER_" + runID
	scriptMarker := "MATERIALIZED_SCRIPT_OK_" + runID

	ownerAuth := agentSessionsRegister(t, ctx, apiBase, ownerEmail, password, "Skills E2E Owner "+runID)
	if len(ownerAuth.Orgs) != 1 || ownerAuth.Orgs[0].ID == "" {
		t.Fatalf("owner register did not return one org: %+v", ownerAuth.Orgs)
	}
	orgID := ownerAuth.Orgs[0].ID
	ownerToken := ownerAuth.AccessToken

	agents := agentSessionsListAgents(t, ctx, apiBase, ownerToken, orgID)
	defaultAgent := findDefaultAgent(t, agents)
	t.Logf("default agent id=%s name=%s", defaultAgent.ID, defaultAgent.Name)

	slug := agentSessionsSeedScriptSkillFixture(t, uuid.MustParse(orgID), uuid.MustParse(defaultAgent.ID), runID)
	t.Logf("seeded script skill slug=%s installed on agent=%s", slug, defaultAgent.ID)

	channels := agentSessionsListChannels(t, ctx, apiBase, ownerToken, orgID)
	general := findDefaultGeneralChannel(t, channels, defaultAgent.ID)

	prompt := strings.Join([]string{
		"This is the Hivy skills materialize E2E. Do these steps in order, one tool call at a time, and do not skip any step:",
		"1) Call skills_list and confirm a skill named \"" + slug + "\" is listed.",
		"2) Call skill_view with name \"" + slug + "\" to load it.",
		"3) Call skill_view with name \"" + slug + "\" and file_path \"references/guide.md\", then read the exact marker token it returns.",
		"4) Run bash exactly once with this command: bash .skills/" + slug + "/scripts/hello.sh",
		"5) Reply with exactly one line, and nothing else, containing these three tokens separated by single spaces: " +
			finalMarker + " then the marker token from step 3 then the exact output line from step 4.",
	}, "\n")

	session := agentSessionsCreateSession(t, ctx, apiBase, ownerToken, orgID, general.ID, prompt)
	if session.Session.ID == "" || session.Session.AgentTurnID == "" {
		t.Fatalf("session was not created with an active turn: %+v", session.Session)
	}
	t.Logf("created session id=%s turn=%s", session.Session.ID, session.Session.AgentTurnID)

	access := fetchAgentSessionsSandboxAccess(t, ctx, apiBase, ownerToken, orgID, session.Session.ID)
	t.Cleanup(func() {
		agentSessionsDeleteSandbox(t, ctx, apiBase, ownerToken, orgID, access.SandboxID)
	})
	stream := agentSessionsStartSandboxStreamFromTurnFollowWithAccess(t, ctx, session.Session.ID, access, session.Session.AgentTurnID)

	events := collectSkillsSandboxEventsUntilTerminal(t, ctx, stream, 15*time.Minute)
	t.Logf("collected %d sandbox stream events", len(events))

	// Discovery + load: the MCP tools are exposed to the model prefixed with the
	// server name (hivy_skills_list / hivy_skill_view), so match on the substring.
	assertSkillsToolCallObserved(t, events, "skills_list")
	assertSkillsToolCallObserved(t, events, "skill_view")

	// Reference read via skill_view(name, file_path).
	assertSkillsStreamContains(t, events, refMarker)
	// Materialized script executed on the real sandbox disk -> its stdout marker.
	assertSkillsStreamContains(t, events, scriptMarker)
	// Task completed as instructed.
	assertSkillsStreamContains(t, events, finalMarker)
}

// agentSessionsSeedScriptSkillFixture creates a plugin + a published skill whose
// bundle ships a linked reference AND a runnable script, then installs the plugin
// on the given agent (populating agent_plugin_installs, which the MCP skill
// resolver keys off). Returns the skill slug. Cleaned up via t.Cleanup.
func agentSessionsSeedScriptSkillFixture(t *testing.T, orgID, agentID uuid.UUID, runID string) string {
	t.Helper()
	db := agentSessionsOpenDB(t)

	pluginID := uuid.New()
	plugin := model.Plugin{
		ID:          pluginID,
		Slug:        "e2e-greeter-plugin-" + runID,
		Name:        "E2E Greeter Plugin " + runID,
		Description: "E2E plugin shipping a skill with a runnable script",
		Category:    "productivity",
		Status:      model.PluginStatusActive,
		Manifest:    model.RawJSON(`{}`),
	}
	if err := db.Create(&plugin).Error; err != nil {
		t.Fatalf("create plugin fixture: %v", err)
	}

	bundle, err := json.Marshal(map[string]any{
		"description": "Greeter skill for the skills materialize E2E.",
		"content":     "# Greeter\nRun scripts/hello.sh to greet. Reference marker lives in references/guide.md.",
		"files": map[string]string{
			"references/guide.md": "REFERENCE_MARKER_" + runID + "\n",
			"scripts/hello.sh":    "#!/usr/bin/env bash\necho MATERIALIZED_SCRIPT_OK_" + runID + "\n",
		},
	})
	if err != nil {
		t.Fatalf("marshal skill bundle: %v", err)
	}

	desc := "Greeter skill that ships a runnable script for the materialize E2E."
	slug := "e2e-greeter-" + runID
	skill := model.Skill{
		ID:          uuid.New(),
		PluginID:    &pluginID,
		Slug:        slug,
		Name:        "E2E Greeter Skill " + runID,
		Description: &desc,
		Category:    "productivity",
		SourceType:  model.SkillSourceInline,
		RepoRef:     "main",
		Bundle:      model.RawJSON(bundle),
		Status:      model.SkillStatusPublished,
	}
	if err := db.Create(&skill).Error; err != nil {
		t.Fatalf("create skill fixture: %v", err)
	}

	install := model.AgentPluginInstall{OrgID: orgID, AgentID: agentID, PluginID: pluginID}
	if err := db.Create(&install).Error; err != nil {
		t.Fatalf("install plugin on agent: %v", err)
	}

	t.Cleanup(func() {
		db.Where("agent_id = ? AND plugin_id = ?", agentID, pluginID).Delete(&model.AgentPluginInstall{})
		db.Where("plugin_id = ?", pluginID).Delete(&model.Skill{})
		db.Where("id = ?", pluginID).Delete(&model.Plugin{})
	})
	return slug
}

// collectSkillsSandboxEventsUntilTerminal drains the live sandbox stream,
// accumulating every event until the turn terminates (or timeout).
func collectSkillsSandboxEventsUntilTerminal(t *testing.T, ctx context.Context, stream *agentSessionsLiveSandboxStream, timeout time.Duration) []runtimeSSEEvent {
	t.Helper()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	all := make([]runtimeSSEEvent, 0, 128)
	for {
		select {
		case event := <-stream.events:
			all = append(all, event)
			if event.Name == "turn_completed" || event.Name == "done" {
				return all
			}
		case events := <-stream.done:
			return append(all, events...)
		case err := <-stream.errs:
			t.Fatalf("sandbox stream failed after %d events: %v", len(all), err)
		case <-ctx.Done():
			t.Fatalf("context ended after %d events: %v", len(all), ctx.Err())
		case <-timer.C:
			t.Fatalf("timed out after %d events; observed=%s", len(all), summarizeRuntimeSSEEvents(all))
		}
	}
}

func assertSkillsToolCallObserved(t *testing.T, events []runtimeSSEEvent, toolSubstring string) {
	t.Helper()
	for _, event := range events {
		if event.Name == "tool_call" && strings.Contains(event.RawData, toolSubstring) {
			return
		}
	}
	t.Fatalf("no tool_call containing %q; observed=%s", toolSubstring, summarizeRuntimeSSEEvents(events))
}

func assertSkillsStreamContains(t *testing.T, events []runtimeSSEEvent, marker string) {
	t.Helper()
	for _, event := range events {
		if strings.Contains(event.RawData, marker) {
			return
		}
	}
	t.Fatalf("marker %q not found in sandbox stream; observed=%s", marker, summarizeRuntimeSSEEvents(events))
}
