package apps

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

// TestAppToolsChannelIsolation pins the session-channel scope: an app in
// another channel of the same org is indistinguishable from a missing one for
// every tool that addresses an app by ID.
func TestAppToolsChannelIsolation(t *testing.T) {
	h, client, session := setupAppTools(t)

	otherApp, err := h.svc.CreateApp(context.Background(), CreateAppParams{
		OrgID:     h.org.ID,
		ChannelID: h.otherChan.ID,
		SheetID:   h.otherSheet.ID,
		Name:      "Other Channel App",
	})
	if err != nil {
		t.Fatalf("create other-channel app: %v", err)
	}

	const want = "app not found in this channel"
	assertAppToolError(t, client, session, toolAppStatus, map[string]any{"app_id": otherApp.ID.String()}, want)
	assertAppToolError(t, client, session, toolAppLogs, map[string]any{"app_id": otherApp.ID.String()}, want)
	assertAppToolError(t, client, session, toolAppRollback, map[string]any{
		"app_id": otherApp.ID.String(), "version_id": uuid.NewString(),
	}, want)
	assertAppToolError(t, client, session, toolAppPublish, map[string]any{
		"app_id":     otherApp.ID.String(),
		"source_key": "pub/e/x/source.zip", "bundle_key": "pub/e/x/bundle.zip",
		"source_sha256": strings.Repeat("a", 64), "bundle_sha256": strings.Repeat("b", 64),
	}, want)

	// A missing app reads identically.
	assertAppToolError(t, client, session, toolAppStatus, map[string]any{"app_id": uuid.NewString()}, want)
}

// TestAppPublishStatusLogsRollbackTools drives the full agent loop against
// the fake appd: publish → status → logs → second publish → rollback.
func TestAppPublishStatusLogsRollbackTools(t *testing.T) {
	h, client, session := setupAppTools(t)
	appd := newFakeAppd(t)
	h.provider.endpoints[appdPort] = appd.server.URL
	h.provider.endpoints[appPort] = "http://127.0.0.1:45678"

	created := callAppTool(t, client, session, toolAppCreate, map[string]any{
		"name": "Flow App", "sheet_id": h.sheet.ID.String(),
	})
	appID := created["app_id"].(string)

	// Publish v1.
	sourceKey, bundleKey, sourceSHA, bundleSHA := h.seedDriveObjects(t, []byte("src-1"), []byte("bin-1"))
	published := callAppTool(t, client, session, toolAppPublish, map[string]any{
		"app_id": appID, "source_key": sourceKey, "bundle_key": bundleKey,
		"source_sha256": sourceSHA, "bundle_sha256": bundleSHA,
		"notes": "first release",
	})
	if published["status"] != string(model.AppStatusRunning) {
		t.Fatalf("publish status = %v", published)
	}
	// The publish URL carries the ?app=<appID> launch hint the frontend uses.
	if published["url"] != "http://127.0.0.1:45678?app="+appID {
		t.Fatalf("publish url = %v", published["url"])
	}
	v1 := mustAppUUID(t, published["version_id"])

	// The version carries agent + session attribution.
	var version model.AppVersion
	if err := h.db.First(&version, "id = ?", v1).Error; err != nil {
		t.Fatalf("load version: %v", err)
	}
	if version.CreatedByAgentID == nil || *version.CreatedByAgentID != h.agent.ID {
		t.Fatalf("version created_by_agent_id = %v", version.CreatedByAgentID)
	}
	if version.SourceSessionID == nil || *version.SourceSessionID != session.ID {
		t.Fatalf("version source_session_id = %v", version.SourceSessionID)
	}

	// Status: lifecycle + url + active version + live appd health.
	status := callAppTool(t, client, session, toolAppStatus, map[string]any{"app_id": appID})
	if status["status"] != string(model.AppStatusRunning) || status["url"] != "http://127.0.0.1:45678" {
		t.Fatalf("app_status = %v", status)
	}
	active, ok := status["active_version"].(map[string]any)
	if !ok || active["id"] != v1.String() || active["notes"] != "first release" {
		t.Fatalf("app_status active_version = %v", status["active_version"])
	}
	health, ok := status["health"].(map[string]any)
	if !ok || health["ok"] != true {
		t.Fatalf("app_status health = %v", status["health"])
	}
	if _, present := status["health_error"]; present {
		t.Fatalf("unexpected health_error: %v", status["health_error"])
	}

	// Logs come back as plain readable text, not JSON.
	logsResult := callAppToolRaw(t, client, session, toolAppLogs, map[string]any{
		"app_id": appID, "lines": 50, "grep": "error",
	})
	if logsResult.IsError {
		t.Fatalf("app_logs errored: %s", appToolResultText(logsResult))
	}
	logsText := appToolResultText(logsResult)
	if !strings.Contains(logsText, "log lines (stream app)") || !strings.Contains(logsText, `"msg":"boom"`) {
		t.Fatalf("app_logs text = %q", logsText)
	}
	if calls := appd.recorded("/logs"); len(calls) != 1 {
		t.Fatalf("appd /logs calls = %d, want 1", len(calls))
	}

	// Publish v2, then roll back to v1.
	h.store.put(sourceKey, []byte("src-2"))
	h.store.put(bundleKey, []byte("bin-2"))
	callAppTool(t, client, session, toolAppPublish, map[string]any{
		"app_id": appID, "source_key": sourceKey, "bundle_key": bundleKey,
		"source_sha256": sha256Hex([]byte("src-2")), "bundle_sha256": sha256Hex([]byte("bin-2")),
		"notes": "second release",
	})
	rolled := callAppTool(t, client, session, toolAppRollback, map[string]any{
		"app_id": appID, "version_id": v1.String(),
	})
	if rolled["status"] != string(model.AppStatusRunning) {
		t.Fatalf("app_rollback = %v", rolled)
	}
	rollbacks := appd.recorded("/rollback")
	if len(rollbacks) != 1 || rollbacks[0].Body["sha256"] != bundleSHA {
		t.Fatalf("appd rollback calls = %+v", rollbacks)
	}
	var app model.App
	if err := h.db.First(&app, "id = ?", appID).Error; err != nil {
		t.Fatalf("reload app: %v", err)
	}
	if app.ActiveVersionID == nil || *app.ActiveVersionID != v1 {
		t.Fatalf("active_version_id after rollback = %v", app.ActiveVersionID)
	}

	// A failing deploy (bundle rejected by appd) surfaces as a clear tool error
	// blaming the app build and pointing at app_logs.
	appd.forceStatus("/deploy", http.StatusUnprocessableEntity)
	h.store.put(sourceKey, []byte("src-3"))
	h.store.put(bundleKey, []byte("bin-3"))
	assertAppToolError(t, client, session, toolAppPublish, map[string]any{
		"app_id": appID, "source_key": sourceKey, "bundle_key": bundleKey,
		"source_sha256": sha256Hex([]byte("src-3")), "bundle_sha256": sha256Hex([]byte("bin-3")),
	}, "app_logs")
}
