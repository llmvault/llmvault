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

// TestAppsRealtimeE2E is the realtime flagship for Hivy apps: it proves that a
// DEPLOYED app receives live sheet changes over SSE, with NO app-builder agent
// and NO LLM anywhere. The harness itself seeds a sheet, builds the template
// directly, publishes it through the REST version endpoint (synchronous
// deploy), performs the launch auth handoff, then drives the realtime pipe end
// to end:
//
//	a. subscribe → an `event: refresh` frame arrives first
//	b. INSERT → rows_changed(insert) with row IDs AND full row snapshots
//	c. UPDATE → rows_changed(update) with patches + a refreshed snapshot
//	d. DELETE → rows_changed(delete) with the row ID and NO snapshots
//	e. cross-sheet isolation: a second sheet's writes never reach the stream
//	f. refetch agrees: the app's own rows/query matches the event-described state
//	g. fan-out: a second concurrent SSE client also gets a later write's event
//
// It also re-verifies (via the flagship helpers) the REST publish DB rows,
// sha-verified S3 artifacts, live HTTP surface, systemd sandbox, and launch
// contract — so the REST publish path is proven end to end too.
func TestAppsRealtimeE2E(t *testing.T) {
	if os.Getenv("HIVY_AGENT_SESSIONS_E2E") != "1" {
		t.Skip("set HIVY_AGENT_SESSIONS_E2E=1 to run against the live compose stack")
	}
	loadEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 28*time.Minute)
	defer cancel()

	apiBase := agentSessionsBaseURL("HIVY_API_BASE_URL", "HIVY_COMPOSE_API_PORT", "8080")
	workerBase := agentSessionsBaseURL("HIVY_WORKER_BASE_URL", "HIVY_COMPOSE_WORKER_HEALTH_PORT", "8090")
	requireAgentSessionsHealthy(t, ctx, apiBase, "api")
	requireAgentSessionsHealthy(t, ctx, workerBase, "worker")

	runID := strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	password := "apps-realtime-e2e-password" //nolint:gosec // test fixture password, not a real credential
	ownerEmail := "apps-realtime-owner-" + runID + "@example.com"
	ownerName := "Apps Realtime Owner " + runID

	// --- Fresh org + owner. The bootstrapped #general channel gives us a
	// channel (and default agent) without provisioning any sandbox/LLM.
	ownerAuth := agentSessionsRegister(t, ctx, apiBase, ownerEmail, password, ownerName)
	if len(ownerAuth.Orgs) != 1 || ownerAuth.Orgs[0].ID == "" {
		t.Fatalf("owner register did not return one org: %+v", ownerAuth.Orgs)
	}
	orgID := ownerAuth.Orgs[0].ID
	token := ownerAuth.AccessToken

	qaAgentInstallPlugin(t, ctx, apiBase, token, orgID, "sheets")
	qaAgentInstallPlugin(t, ctx, apiBase, token, orgID, "apps")
	t.Log("installed sheets + apps org plugins")

	channelID := appsRealtimeDefaultChannel(t, ctx, apiBase, token, orgID)
	t.Logf("using channel id=%s", channelID)

	// --- Seed the bound sheet (page + fields + a couple rows), all over REST.
	bound := appsRealtimeCreateSheet(t, ctx, apiBase, token, orgID, channelID,
		"Team Directory "+runID, []string{"Name", "Role", "Email"})
	appsRealtimeInsertRows(t, ctx, apiBase, token, orgID, bound, []map[string]any{
		{bound.FieldIDs["Name"]: "Seed One", bound.FieldIDs["Role"]: "Founder", bound.FieldIDs["Email"]: "one@example.com"},
		{bound.FieldIDs["Name"]: "Seed Two", bound.FieldIDs["Role"]: "Ops", bound.FieldIDs["Email"]: "two@example.com"},
	})
	t.Logf("bound sheet id=%s page=%s seeded", bound.SheetID, bound.PageID)

	// --- Create the app bound to that sheet (REST).
	appID := appsRealtimeCreateApp(t, ctx, apiBase, token, orgID, channelID, bound.SheetID, "Realtime "+runID)
	t.Logf("created app id=%s", appID)

	// --- Build the template DIRECTLY (no agent) and publish via REST.
	sourceZip, bundleZip := appsRealtimeBuildTemplate(t, ctx)
	publishStart := time.Now()
	pub := appsRealtimePublish(t, ctx, apiBase, token, orgID, appID, sourceZip, bundleZip)
	t.Logf("REST publish → 201 version=%s status=%s url=%s (deploy took %s)",
		pub.VersionID, pub.Status, pub.URL, time.Since(publishStart).Round(time.Second))

	// --- Reuse the flagship pillars to prove the REST publish landed: DB rows,
	// sha-verified S3 artifacts, live HTTP surface, systemd sandbox, launch auth.
	db := agentSessionsOpenDB(t)
	app, version := appsFlagshipLoadDeployedApp(t, ctx, db, orgID, channelID)
	if app.ID.String() != appID {
		t.Fatalf("deployed app id=%s want %s", app.ID, appID)
	}
	assertAppsFlagshipObjects(t, ctx, app, version)
	appBase := assertAppsFlagshipHTTPSurface(t, ctx, db, app)
	assertAppsFlagshipSystemdSandbox(t, ctx, db, app)
	cookie := assertAppsFlagshipLaunchHandoff(t, ctx, apiBase, appBase, token, orgID, app, appsFlagshipOwner{
		UserID: ownerAuth.User.ID, Email: ownerEmail, Name: ownerName,
	})
	t.Logf("REST publish + auth handoff verified; app live at %s", appBase)

	// --- THE REALTIME PILLARS.
	appsRealtimeAssertPillars(t, ctx, apiBase, appBase, token, orgID, channelID, cookie, runID, bound)

	t.Logf("apps realtime E2E passed: app live at %s (container-bound sheet %s)", appBase, bound.SheetID)
}

// appsRealtimeDefaultChannel returns the org's bootstrapped default channel ID.
func appsRealtimeDefaultChannel(t *testing.T, ctx context.Context, apiBase, token, orgID string) string {
	t.Helper()
	channels := agentSessionsListChannels(t, ctx, apiBase, token, orgID)
	for _, c := range channels {
		if c.IsDefault && c.ID != "" {
			return c.ID
		}
	}
	if len(channels) > 0 && channels[0].ID != "" {
		return channels[0].ID
	}
	t.Fatalf("no channel found in org %s", orgID)
	return ""
}

// appsRealtimeCreateApp registers an app bound to one sheet in one channel.
func appsRealtimeCreateApp(t *testing.T, ctx context.Context, apiBase, token, orgID, channelID, sheetID, name string) string {
	t.Helper()
	var out struct {
		ID string `json:"id"`
	}
	agentSessionsJSON(t, ctx, http.MethodPost, apiBase+"/v1/apps", token, orgID, map[string]any{
		"channel_id": channelID,
		"sheet_id":   sheetID,
		"name":       name,
	}, http.StatusCreated, &out)
	if out.ID == "" {
		t.Fatalf("create app returned empty id")
	}
	return out.ID
}
