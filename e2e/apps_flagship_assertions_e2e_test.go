package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

var appsFlagshipSHARe = regexp.MustCompile(`^[a-f0-9]{64}$`)

// appsFlagshipLoadDeployedApp asserts pillar (a): the org's one app is
// running with an active version whose row carries sha256s and immutable
// pub/o object keys, and exactly ONE version row exists (a single authorized
// publish).
func appsFlagshipLoadDeployedApp(t *testing.T, ctx context.Context, db *gorm.DB, orgID, channelID string) (*model.App, *model.AppVersion) {
	t.Helper()
	var apps []model.App
	if err := db.WithContext(ctx).Where("org_id = ? AND archived_at IS NULL", uuid.MustParse(orgID)).Find(&apps).Error; err != nil {
		t.Fatalf("load apps: %v", err)
	}
	if len(apps) != 1 {
		t.Fatalf("expected exactly 1 active app in org, got %d: %+v", len(apps), apps)
	}
	app := apps[0]
	if app.ChannelID.String() != channelID {
		t.Fatalf("app channel_id=%s want %s", app.ChannelID, channelID)
	}
	if app.Status != model.AppStatusRunning {
		t.Fatalf("app status=%q want running", app.Status)
	}
	if app.ActiveVersionID == nil {
		t.Fatalf("app has no active_version_id")
	}
	if app.SandboxID == nil {
		t.Fatalf("app has no sandbox_id")
	}

	var versions []model.AppVersion
	if err := db.WithContext(ctx).Where("app_id = ?", app.ID).Find(&versions).Error; err != nil {
		t.Fatalf("load app versions: %v", err)
	}
	if len(versions) != 1 {
		t.Fatalf("expected exactly 1 app_versions row (one authorized publish), got %d: %+v", len(versions), versions)
	}
	version := versions[0]
	if version.ID != *app.ActiveVersionID {
		t.Fatalf("active_version_id=%s does not match the only version row %s", *app.ActiveVersionID, version.ID)
	}
	if !appsFlagshipSHARe.MatchString(version.SourceSHA256) || !appsFlagshipSHARe.MatchString(version.BundleSHA256) {
		t.Fatalf("version sha256s malformed: source=%q bundle=%q", version.SourceSHA256, version.BundleSHA256)
	}
	wantPrefix := fmt.Sprintf("pub/o/%s/apps/%s/%s/", orgID, app.Slug, version.BundleSHA256)
	if version.SourceObjectKey != wantPrefix+"source.zip" {
		t.Fatalf("source object key %q want %q", version.SourceObjectKey, wantPrefix+"source.zip")
	}
	if version.BundleObjectKey != wantPrefix+"bundle.zip" {
		t.Fatalf("bundle object key %q want %q", version.BundleObjectKey, wantPrefix+"bundle.zip")
	}
	if version.SourceBytes <= 0 || version.BundleBytes <= 0 {
		t.Fatalf("version byte sizes not recorded: source=%d bundle=%d", version.SourceBytes, version.BundleBytes)
	}
	return &app, &version
}

// assertAppsFlagshipObjects is pillar (b): both S3 objects exist, are
// non-empty, and hash to exactly the shas the version row recorded.
func assertAppsFlagshipObjects(t *testing.T, ctx context.Context, app *model.App, version *model.AppVersion) {
	t.Helper()
	store := appsFlagshipObjectStore(t)
	sourceSHA, sourceBytes := appsFlagshipObjectSHA(t, ctx, store, version.SourceObjectKey)
	if sourceBytes <= 0 || sourceBytes != version.SourceBytes || sourceSHA != version.SourceSHA256 {
		t.Fatalf("source object mismatch: bytes=%d (want %d) sha=%s (want %s)", sourceBytes, version.SourceBytes, sourceSHA, version.SourceSHA256)
	}
	bundleSHA, bundleBytes := appsFlagshipObjectSHA(t, ctx, store, version.BundleObjectKey)
	if bundleBytes <= 0 || bundleBytes != version.BundleBytes || bundleSHA != version.BundleSHA256 {
		t.Fatalf("bundle object mismatch: bytes=%d (want %d) sha=%s (want %s)", bundleBytes, version.BundleBytes, bundleSHA, version.BundleSHA256)
	}
	t.Logf("S3 artifacts verified: source %d bytes sha=%s, bundle %d bytes sha=%s for app %s", sourceBytes, sourceSHA, bundleBytes, bundleSHA, app.ID)
}

// assertAppsFlagshipHTTPSurface is pillar (c) for the DEPLOYED app: resolve
// the app sandbox's published 8080 port the way GetEndpoint does, then
// require /healthz and the unauthenticated 401 page. Returns the
// host-reachable app base URL.
func assertAppsFlagshipHTTPSurface(t *testing.T, ctx context.Context, db *gorm.DB, app *model.App) string {
	t.Helper()
	sb := appsFlagshipLoadSandbox(t, ctx, db, *app.SandboxID)
	port := appsFlagshipContainerHostPort(t, ctx, sb.ExternalID, 8080)
	appBase := "http://127.0.0.1:" + port
	t.Logf("app sandbox container=%s serves on %s", sb.ExternalID, appBase)

	assertAppsFlagshipHealthz(t, ctx, appBase, app.TemplateVersion, "deployed")
	assertAppsFlagshipUnauthPage(t, ctx, appBase, "deployed")
	return appBase
}

// appsFlagshipLoadSandbox loads and sanity-checks the app sandbox row.
func appsFlagshipLoadSandbox(t *testing.T, ctx context.Context, db *gorm.DB, sandboxID uuid.UUID) *model.Sandbox {
	t.Helper()
	var sb model.Sandbox
	if err := db.WithContext(ctx).Where("id = ?", sandboxID).First(&sb).Error; err != nil {
		t.Fatalf("load app sandbox row: %v", err)
	}
	if sb.ProviderID != "docker" || strings.TrimSpace(sb.ExternalID) == "" || sb.Status != "running" {
		t.Fatalf("app sandbox row unexpected: provider=%q external_id=%q status=%q", sb.ProviderID, sb.ExternalID, sb.Status)
	}
	return &sb
}

// assertAppsFlagshipSheetData is pillar (e): the bound sheet holds the
// seeded team-member rows and the authenticated SPA shell serves. Returns a
// bound-sheet page ID (used by the binding-security pillar).
func assertAppsFlagshipSheetData(t *testing.T, ctx context.Context, apiBase, appBase, token, orgID string, app *model.App, cookie string) string {
	t.Helper()
	var structure struct {
		Sheet struct {
			Name string `json:"name"`
		} `json:"sheet"`
		Pages []struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			RowCount int64  `json:"row_count"`
		} `json:"pages"`
	}
	agentSessionsJSON(t, ctx, http.MethodGet, apiBase+"/v1/sheets/"+app.SheetID.String(), token, orgID, nil, http.StatusOK, &structure)
	totalRows := int64(0)
	boundPageID := ""
	for _, page := range structure.Pages {
		t.Logf("bound sheet %q page %q rows=%d", structure.Sheet.Name, page.Name, page.RowCount)
		totalRows += page.RowCount
		if boundPageID == "" || page.RowCount > 0 {
			boundPageID = page.ID
		}
	}
	if totalRows < 1 || boundPageID == "" {
		t.Fatalf("bound sheet %s has no rows: %+v", app.SheetID, structure)
	}
	if totalRows < 3 {
		t.Logf("note: only %d seeded row(s) — asked for three team members", totalRows)
	}

	spa := appsFlagshipDo(t, ctx, http.MethodGet, appBase+"/", map[string]string{
		"Accept": "text/html",
		"Cookie": "hivy_app_session=" + cookie,
	}, nil)
	if spa.Status != http.StatusOK {
		t.Fatalf("authenticated GET / status=%d want 200; body=%s", spa.Status, spa.Body)
	}
	if !strings.Contains(string(spa.Body), `id="root"`) {
		t.Fatalf("authenticated / did not serve the Vite SPA shell; body=%s", spa.Body)
	}
	assertAppsFlagshipCSP(t, spa.Header, "spa")
	t.Logf("authenticated SPA shell served (%d bytes); bound sheet holds %d rows", len(spa.Body), totalRows)
	return boundPageID
}

// assertAppsFlagshipLogsToolResult is pillar (f): the logs turn must have
// produced a persisted app_logs tool result whose summary carries real
// production log lines (the JSON access/startup lines hivycore writes).
func assertAppsFlagshipLogsToolResult(t *testing.T, ctx context.Context, apiBase, token, orgID, sessionID string, app *model.App) {
	t.Helper()
	// The runtime namespaces Hivy MCP tools (persisted tool name is
	// "hivy_app_logs"); match on the canonical suffix.
	event := appsFlagshipFindEvent(t, ctx, apiBase, token, orgID, sessionID, func(e agentSessionsEvent) bool {
		return e.EventType == "tool_result" && strings.HasSuffix(eventString(e.Payload, "tool"), "app_logs") && eventString(e.Payload, "status") == "completed"
	})
	if event == nil {
		t.Fatalf("no completed app_logs tool_result event found in session %s", sessionID)
	}
	args := eventString(event.Payload, "args_summary")
	if !strings.Contains(args, app.ID.String()) {
		t.Fatalf("app_logs args did not target app %s: %s", app.ID, args)
	}
	summary := eventString(event.Payload, "result_summary")
	if !strings.Contains(summary, "log lines (stream") {
		t.Fatalf("app_logs result has no log payload header: %s", summary)
	}
	if !strings.Contains(summary, "request") && !strings.Contains(summary, "listening") {
		t.Fatalf("app_logs result carries no recognizable production log lines: %s", summary)
	}
	t.Logf("app_logs tool result verified (event %s): %.300s…", event.ID, summary)
}

// assertAppsFlagshipSheetBinding is pillar (g): with the app's decrypted
// runtime secret, the internal app API serves the bound sheet's rows but
// 404s a page of a DIFFERENT sheet, and rejects a wrong secret outright.
func assertAppsFlagshipSheetBinding(t *testing.T, ctx context.Context, apiBase, token, orgID, channelID string, app *model.App, boundPageID string) {
	t.Helper()
	secret := appsFlagshipDecryptAppSecret(t, app.EncryptedAppSecret)
	if !strings.HasPrefix(secret, model.AppSecretPrefix) {
		t.Fatalf("decrypted app secret has prefix %q want %q", secret[:min(len(secret), 6)], model.AppSecretPrefix)
	}
	internalBase := apiBase + "/internal/apps/" + app.ID.String() + "/v1"
	appAuth := map[string]string{"Authorization": "Bearer " + secret, "Content-Type": "application/json"}

	// Positive control: the bound sheet's rows flow through the app secret.
	bound := appsFlagshipDo(t, ctx, http.MethodPost, internalBase+"/pages/"+boundPageID+"/rows/query", appAuth, []byte(`{}`))
	if bound.Status != http.StatusOK {
		t.Fatalf("internal app API bound-page query status=%d body=%s", bound.Status, bound.Body)
	}
	var rows struct {
		Rows []json.RawMessage `json:"rows"`
	}
	if err := json.Unmarshal(bound.Body, &rows); err != nil || len(rows.Rows) < 1 {
		t.Fatalf("internal app API returned no bound-sheet rows: err=%v body=%s", err, bound.Body)
	}
	t.Logf("internal app API served %d bound-sheet rows with the app secret", len(rows.Rows))

	// A page of a DIFFERENT sheet in the same org/channel must 404.
	var decoy struct {
		Pages []struct {
			ID string `json:"id"`
		} `json:"pages"`
	}
	agentSessionsJSON(t, ctx, http.MethodPost, apiBase+"/v1/sheets", token, orgID, map[string]any{
		"channel_id": channelID,
		"name":       "Binding Decoy " + app.Slug,
		"pages": []map[string]any{{
			"name":   "Main",
			"fields": []map[string]any{{"name": "Name", "type": "text"}},
		}},
	}, http.StatusCreated, &decoy)
	if len(decoy.Pages) == 0 || decoy.Pages[0].ID == "" {
		t.Fatalf("decoy sheet create returned no pages: %+v", decoy)
	}
	cross := appsFlagshipDo(t, ctx, http.MethodPost, internalBase+"/pages/"+decoy.Pages[0].ID+"/rows/query", appAuth, []byte(`{}`))
	if cross.Status != http.StatusNotFound {
		t.Fatalf("SECURITY: internal app API answered a foreign sheet's page with status=%d body=%s", cross.Status, cross.Body)
	}
	t.Log("internal app API 404s a page outside the bound sheet")

	// A wrong secret never authenticates.
	badAuth := map[string]string{"Authorization": "Bearer " + model.AppSecretPrefix + strings.Repeat("0", 64), "Content-Type": "application/json"}
	unauth := appsFlagshipDo(t, ctx, http.MethodPost, internalBase+"/pages/"+boundPageID+"/rows/query", badAuth, []byte(`{}`))
	if unauth.Status != http.StatusUnauthorized {
		t.Fatalf("SECURITY: wrong app secret got status=%d body=%s", unauth.Status, unauth.Body)
	}
	t.Log("internal app API rejects a wrong app secret with 401")
}
