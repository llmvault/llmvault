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

// TestAgentSessionsAppBuilderFlagshipE2E is the acceptance gate for the Apps
// feature (docs/apps-plan.md), asserting the preview-first workflow with a
// REAL app-builder catalog agent on a real LLM in a real docker sandbox:
//
//	ACT 1 — the user asks for a PREVIEW and explicitly withholds deploy
//	authorization. The agent seeds a sheet, scaffolds the template, builds
//	the app, and runs `make preview` in its own sandbox. The test proves the
//	authorization gate held (no version rows, app not running), the preview
//	URL was registered and shared, and the preview serves the new iframe
//	auth surface (401 page, CSP, no redirects).
//
//	ACT 2 — the user explicitly authorizes deployment. The agent publishes
//	through drive → app_publish, and the test proves every pillar: DB rows,
//	sha-verified immutable S3 artifacts, the live HTTP surface, the JSON
//	launch contract with claim-by-claim token verification, the partitioned
//	session cookie, one-time-token replay rejection, sheet-data flow, the
//	one-sheet binding boundary, and that the app sandbox booted with systemd
//	as PID 1 supervising hivy-appd.
//
//	ACT 3 — agent-driven production debugging: a third turn reads the app's
//	logs via app_logs and the persisted tool result carries real log lines.
func TestAgentSessionsAppBuilderFlagshipE2E(t *testing.T) {
	if os.Getenv("HIVY_AGENT_SESSIONS_E2E") != "1" {
		t.Skip("set HIVY_AGENT_SESSIONS_E2E=1 to run against the live compose stack")
	}
	loadEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 56*time.Minute)
	defer cancel()

	apiBase := agentSessionsBaseURL("HIVY_API_BASE_URL", "HIVY_COMPOSE_API_PORT", "8080")
	workerBase := agentSessionsBaseURL("HIVY_WORKER_BASE_URL", "HIVY_COMPOSE_WORKER_HEALTH_PORT", "8090")
	requireAgentSessionsHealthy(t, ctx, apiBase, "api")
	requireAgentSessionsHealthy(t, ctx, workerBase, "worker")
	agentSessionsEnsureSystemOpenRouterCredential(t)

	runID := strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	password := "agent-sessions-e2e-password"
	ownerEmail := "app-builder-owner-" + runID + "@example.com"
	ownerName := "App Builder Owner " + runID
	previewMarker := "APP_PREVIEW_E2E_DONE_" + runID
	deployMarker := "APP_DEPLOY_E2E_DONE_" + runID
	logsMarker := "APP_LOGS_E2E_DONE_" + runID

	// --- Fresh org + owner (re-runnable: nothing here depends on prior state).
	t.Logf("registering owner=%s", ownerEmail)
	ownerAuth := agentSessionsRegister(t, ctx, apiBase, ownerEmail, password, ownerName)
	if len(ownerAuth.Orgs) != 1 || ownerAuth.Orgs[0].ID == "" {
		t.Fatalf("owner register did not return one org: %+v", ownerAuth.Orgs)
	}
	orgID := ownerAuth.Orgs[0].ID
	token := ownerAuth.AccessToken

	// --- Org plugins the catalog agent requires, then the agent itself.
	qaAgentInstallPlugin(t, ctx, apiBase, token, orgID, "sheets")
	qaAgentInstallPlugin(t, ctx, apiBase, token, orgID, "apps")
	t.Log("installed sheets + apps org plugins")

	agent := agentSessionsInstallCatalogAgent(t, ctx, apiBase, token, orgID, "ricky-app-builder")
	assertAgentSessionsAgentSandboxImage(t, "app-builder", agent, model.SandboxImageDeveloper)
	t.Logf("installed catalog agent id=%s name=%s model=%s sandbox_image=%s", agent.ID, agent.Name, agent.Model, agent.SandboxImage)

	channel := agentSessionsCreateChannel(t, ctx, apiBase, token, orgID, "apps-"+runID, agent.ID)
	if channel.DefaultAgentID != agent.ID {
		t.Fatalf("channel default_agent_id=%s want %s", channel.DefaultAgentID, agent.ID)
	}
	t.Logf("created channel id=%s", channel.ID)

	// ════ ACT 1 — build + preview, deploy authorization explicitly WITHHELD.
	previewMessage := strings.Join([]string{
		"We keep a small team directory. Please create a sheet of our team members with a few seeded rows — name, role, and email for three people is enough (make the people up).",
		"Then build a Hivy app called \"Welcome\" that shows the heading \"Welcome to Hivy\" and lists the team members from that sheet.",
		"Don't deploy anything yet — just build the app and give me a preview link to try first; I want to review it before anything ships.",
		"When the preview is up and you've shared its link, end your final reply with " + previewMarker + " and nothing after it.",
	}, " ")
	session := agentSessionsCreateSession(t, ctx, apiBase, token, orgID, channel.ID, previewMessage)
	if session.Session.ID == "" {
		t.Fatalf("session was not created correctly: %+v", session)
	}
	t.Logf("created preview session id=%s queued=%t", session.Session.ID, session.Queued)

	previewStart := time.Now()
	previewFinal := appsFlagshipWaitForFinalMarker(t, ctx, apiBase, &token, orgID, session.Session.ID, previewMarker, ownerEmail, password, 30*time.Minute, 0)
	t.Logf("preview turn completed in %s (final event id=%s seq=%d)", time.Since(previewStart).Round(time.Second), previewFinal.ID, previewFinal.SequenceNumber)

	// --- The authorization gate held: an app record exists, but nothing was
	// published — no version rows, no active version, not running.
	db := agentSessionsOpenDB(t)
	app := appsFlagshipLoadPreviewApp(t, ctx, db, orgID, channel.ID)
	t.Logf("app id=%s slug=%s status=%s sheet=%s preview_url=%s", app.ID, app.Slug, app.Status, app.SheetID, app.PreviewURL)
	assertAppsFlagshipNoPublishAttempt(t, ctx, apiBase, token, orgID, session.Session.ID)

	// --- The agent handed the user the registered preview URL.
	assertAppsFlagshipFinalMentionsPreview(t, previewFinal, app.PreviewURL)

	// --- The preview answers over HTTP from the host with the new iframe
	// auth surface: /healthz, and 401 HTML (never a redirect) when signed out.
	previewBase := appsFlagshipRewriteHost(t, app.PreviewURL)
	assertAppsFlagshipHealthz(t, ctx, previewBase, "", "preview")
	assertAppsFlagshipUnauthPage(t, ctx, previewBase, "preview")

	// ════ ACT 2 — explicit authorization → deploy.
	deployMessage := "The preview looks great — ship it. Deploy the app and end your final reply with " + deployMarker + " and nothing after it."
	deployTurn := agentSessionsSendMessage(t, ctx, apiBase, token, orgID, session.Session.ID, deployMessage)
	t.Logf("sent deploy turn queued=%t", deployTurn.Queued)
	deployStart := time.Now()
	deployFinal := appsFlagshipWaitForFinalMarker(t, ctx, apiBase, &token, orgID, session.Session.ID, deployMarker, ownerEmail, password, 16*time.Minute, previewFinal.SequenceNumber)
	t.Logf("deploy turn completed in %s (final event id=%s)", time.Since(deployStart).Round(time.Second), deployFinal.ID)

	// --- (a) apps + app_versions rows are real and complete.
	deployedApp, version := appsFlagshipLoadDeployedApp(t, ctx, db, orgID, channel.ID)
	if deployedApp.ID != app.ID {
		t.Fatalf("deploy created a different app: %s want %s", deployedApp.ID, app.ID)
	}
	app = deployedApp
	t.Logf("app id=%s slug=%s status=%s version=%s", app.ID, app.Slug, app.Status, version.ID)

	// --- (b) the immutable S3 artifacts exist, byte-for-byte sha-verified.
	assertAppsFlagshipObjects(t, ctx, app, version)

	// --- (c) the deployed app answers over HTTP from the host: /healthz plus
	// the 401 page (NOT a redirect) with the CSP embedding policy.
	appBase := assertAppsFlagshipHTTPSurface(t, ctx, db, app)

	// --- the app sandbox booted with systemd as PID 1 and hivy-appd is a
	// live systemd service.
	assertAppsFlagshipSystemdSandbox(t, ctx, db, app)

	// --- (d) the JSON launch contract: 200 {token, …}, claim-by-claim token
	// verification, callback → partitioned session cookie → /api/me, replay
	// rejected with the 401 page and no cookie.
	cookie := assertAppsFlagshipLaunchHandoff(t, ctx, apiBase, appBase, token, orgID, app, appsFlagshipOwner{
		UserID: ownerAuth.User.ID, Email: ownerEmail, Name: ownerName,
	})

	// --- (e) sheet data flows: the bound sheet has the seeded rows and the
	// authenticated SPA shell serves.
	boundPageID := assertAppsFlagshipSheetData(t, ctx, apiBase, appBase, token, orgID, app, cookie)

	// --- (g) sheet-binding security on the internal app API.
	assertAppsFlagshipSheetBinding(t, ctx, apiBase, token, orgID, channel.ID, app, boundPageID)

	// ════ ACT 3 — agent-driven production debugging via app_logs.
	logsMessage := "Now fetch the Welcome app's production logs with your app tools and summarize what you see — startup lines, recent requests, anything notable. End your final reply with " + logsMarker + " and nothing after it."
	logsTurn := agentSessionsSendMessage(t, ctx, apiBase, token, orgID, session.Session.ID, logsMessage)
	t.Logf("sent logs turn queued=%t", logsTurn.Queued)
	logsStart := time.Now()
	logsFinal := appsFlagshipWaitForFinalMarker(t, ctx, apiBase, &token, orgID, session.Session.ID, logsMarker, ownerEmail, password, 8*time.Minute, deployFinal.SequenceNumber)
	t.Logf("logs turn completed in %s (final event id=%s)", time.Since(logsStart).Round(time.Second), logsFinal.ID)
	assertAppsFlagshipLogsToolResult(t, ctx, apiBase, token, orgID, session.Session.ID, app)

	t.Logf("apps flagship E2E passed: app %q live at %s, total runtime %s", app.Name, appBase, time.Since(previewStart).Round(time.Second))
}

// appsFlagshipOwner carries the registered owner's identity for the auth
// handoff assertions.
type appsFlagshipOwner struct {
	UserID string
	Email  string
	Name   string
}
