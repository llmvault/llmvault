package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

// appsFlagshipLoadPreviewApp asserts the ACT 1 state: exactly one active app
// in the org (app_create ran), channel- and sheet-bound — but the deployment
// authorization gate HELD: not running, no active version, ZERO app_versions
// rows. The preview side channel registered a preview URL.
func appsFlagshipLoadPreviewApp(t *testing.T, ctx context.Context, db *gorm.DB, orgID, channelID string) *model.App {
	t.Helper()
	var apps []model.App
	if err := db.WithContext(ctx).Where("org_id = ? AND archived_at IS NULL", uuid.MustParse(orgID)).Find(&apps).Error; err != nil {
		t.Fatalf("load apps: %v", err)
	}
	if len(apps) != 1 {
		t.Fatalf("expected exactly 1 active app in org, got %d: %+v", len(apps), apps)
	}
	app := apps[0]
	if !strings.Contains(strings.ToLower(app.Name), "welcome") {
		t.Fatalf("app name %q does not mention Welcome", app.Name)
	}
	if app.ChannelID.String() != channelID {
		t.Fatalf("app channel_id=%s want %s", app.ChannelID, channelID)
	}
	if app.SheetID == uuid.Nil {
		t.Fatalf("app has no bound sheet")
	}
	if len(app.EncryptedAppSecret) == 0 {
		t.Fatalf("app has no encrypted secret")
	}
	if app.CreatedByAgentID == nil || app.SourceSessionID == nil {
		t.Fatalf("app provenance missing: created_by_agent_id=%v source_session_id=%v", app.CreatedByAgentID, app.SourceSessionID)
	}

	// THE AUTHORIZATION GATE: nothing may have shipped.
	if app.Status == model.AppStatusRunning || app.Status == model.AppStatusDeploying {
		t.Fatalf("AUTHORIZATION GATE BROKEN: app status=%q before the user authorized deployment", app.Status)
	}
	if app.ActiveVersionID != nil {
		t.Fatalf("AUTHORIZATION GATE BROKEN: app has active_version_id=%s before authorization", *app.ActiveVersionID)
	}
	var versionCount int64
	if err := db.WithContext(ctx).Model(&model.AppVersion{}).Where("app_id = ?", app.ID).Count(&versionCount).Error; err != nil {
		t.Fatalf("count app versions: %v", err)
	}
	if versionCount != 0 {
		t.Fatalf("AUTHORIZATION GATE BROKEN: %d app_versions row(s) exist before authorization", versionCount)
	}
	t.Logf("authorization gate held: status=%q, no active version, zero app_versions rows", app.Status)

	// The preview side channel ran: make preview registered the preview URL.
	if strings.TrimSpace(app.PreviewURL) == "" {
		t.Fatalf("apps.preview_url is empty — make preview never registered a preview")
	}
	if app.PreviewRegisteredAt == nil {
		t.Fatalf("apps.preview_registered_at is nil despite preview_url=%q", app.PreviewURL)
	}
	return &app
}

// assertAppsFlagshipNoPublishAttempt scans the whole session transcript for
// any app_publish tool activity — the agent must not even have TRIED to
// deploy in act 1.
func assertAppsFlagshipNoPublishAttempt(t *testing.T, ctx context.Context, apiBase, token, orgID, sessionID string) {
	t.Helper()
	event := appsFlagshipFindEvent(t, ctx, apiBase, token, orgID, sessionID, func(e agentSessionsEvent) bool {
		return strings.HasSuffix(eventString(e.Payload, "tool"), "app_publish")
	})
	if event != nil {
		payload, _ := json.Marshal(event.Payload)
		t.Fatalf("AUTHORIZATION GATE BROKEN: the agent attempted app_publish in the preview turn (event %s type=%s): %s", event.ID, event.EventType, payload)
	}
	t.Log("no app_publish tool activity anywhere in the preview turn")
}

// assertAppsFlagshipFinalMentionsPreview requires the agent's final preview
// reply to hand the user the registered preview URL.
func assertAppsFlagshipFinalMentionsPreview(t *testing.T, final agentSessionsEvent, previewURL string) {
	t.Helper()
	payload, _ := json.Marshal(final.Payload)
	if !strings.Contains(string(payload), previewURL) {
		t.Fatalf("final preview reply does not contain the registered preview URL %q: %s", previewURL, payload)
	}
	t.Logf("agent's final reply shared the preview URL %s", previewURL)
}

// assertAppsFlagshipHealthz probes {base}/healthz: 200, status ok, and a
// template_version that matches the apps row when the row has one.
func assertAppsFlagshipHealthz(t *testing.T, ctx context.Context, base, wantTemplateVersion, label string) {
	t.Helper()
	health := appsFlagshipDo(t, ctx, http.MethodGet, base+"/healthz", nil, nil)
	if health.Status != http.StatusOK {
		t.Fatalf("[%s] GET /healthz status=%d body=%s", label, health.Status, health.Body)
	}
	var healthBody struct {
		Status          string `json:"status"`
		TemplateVersion string `json:"template_version"`
	}
	if err := json.Unmarshal(health.Body, &healthBody); err != nil {
		t.Fatalf("[%s] decode /healthz: %v body=%s", label, err, health.Body)
	}
	if healthBody.Status != "ok" || healthBody.TemplateVersion == "" {
		t.Fatalf("[%s] /healthz payload unexpected: %+v", label, healthBody)
	}
	if wantTemplateVersion != "" && healthBody.TemplateVersion != wantTemplateVersion {
		t.Fatalf("[%s] /healthz template_version=%q but apps row has %q", label, healthBody.TemplateVersion, wantTemplateVersion)
	}
	t.Logf("[%s] /healthz ok template_version=%s", label, healthBody.TemplateVersion)
}

// assertAppsFlagshipUnauthPage asserts the iframe-model unauthenticated
// surface: GET / with Accept: text/html answers 401 with the built-in "not
// signed in" HTML page — NO redirect of any kind — carrying a plain link to
// the Hivy launch URL, and the CSP frame-ancestors policy naming the Hivy
// frontend origin.
func assertAppsFlagshipUnauthPage(t *testing.T, ctx context.Context, base, label string) {
	t.Helper()
	root := appsFlagshipDo(t, ctx, http.MethodGet, base+"/", map[string]string{"Accept": "text/html"}, nil)
	if root.Status != http.StatusUnauthorized {
		t.Fatalf("[%s] unauthenticated GET / status=%d want 401; body=%s", label, root.Status, root.Body)
	}
	if loc := root.Header.Get("Location"); loc != "" {
		t.Fatalf("[%s] unauthenticated GET / must not redirect, got Location=%q", label, loc)
	}
	if ct := root.Header.Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("[%s] 401 page Content-Type=%q want text/html", label, ct)
	}
	body := string(root.Body)
	if !strings.Contains(body, "not signed in") {
		t.Fatalf("[%s] 401 body is not the not-signed-in page: %s", label, body)
	}
	launch := appsFlagshipFrontendLaunchURL()
	if !strings.Contains(body, `href="`+launch+`"`) {
		t.Fatalf("[%s] 401 page has no launch link to %q: %s", label, launch, body)
	}
	assertAppsFlagshipCSP(t, root.Header, label)
	t.Logf("[%s] unauthenticated / served the 401 page with launch link %s", label, launch)
}

// assertAppsFlagshipCSP requires the frame-ancestors embedding policy on an
// HTML response: 'self' plus the Hivy frontend origin.
func assertAppsFlagshipCSP(t *testing.T, header http.Header, label string) {
	t.Helper()
	csp := header.Get("Content-Security-Policy")
	if !strings.Contains(csp, "frame-ancestors 'self'") {
		t.Fatalf("[%s] CSP missing frame-ancestors: %q", label, csp)
	}
	frontendOrigin := strings.TrimRight(envOr("HIVY_FRONTEND_URL", "http://localhost:30112"), "/")
	if !strings.Contains(csp, frontendOrigin) {
		t.Fatalf("[%s] CSP frame-ancestors does not allow the Hivy frontend %q: %q", label, frontendOrigin, csp)
	}
}

// appsFlagshipFrontendLaunchURL is where the app's 401 page links and what
// the CSP embedding policy derives from: apps open inside the session
// workspace, so the platform points HIVY_LAUNCH_URL at the frontend root
// (internal/apps/env.go launchURL).
func appsFlagshipFrontendLaunchURL() string {
	return strings.TrimRight(envOr("HIVY_FRONTEND_URL", "http://localhost:30112"), "/") + "/"
}
