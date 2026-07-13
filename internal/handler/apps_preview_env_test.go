package handler_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/apps"
	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sandbox"
)

type previewEnvResponse struct {
	PreviewURL string            `json:"preview_url"`
	Env        map[string]string `json:"env"`
}

func TestAppPreviewEnvAuthAndValidation(t *testing.T) {
	h := newPreviewEnvHarness(t, previewHarnessOpts{})
	h.installAppsPlugin(t)
	appID := h.app.ID.String()

	if rr := h.fetch(t, "", appID, "3000"); rr.Code != 401 {
		t.Fatalf("missing bearer status = %d", rr.Code)
	}
	if rr := h.fetch(t, "wrong-secret", appID, "3000"); rr.Code != 401 {
		t.Fatalf("wrong bearer status = %d", rr.Code)
	}
	if rr := h.fetch(t, h.secret, appID, ""); rr.Code != 400 {
		t.Fatalf("missing port status = %d", rr.Code)
	}
	if rr := h.fetch(t, h.secret, appID, "70000"); rr.Code != 400 {
		t.Fatalf("out-of-range port status = %d", rr.Code)
	}
	if rr := h.fetch(t, h.secret, "not-a-uuid", "3000"); rr.Code != 400 {
		t.Fatalf("bad app id status = %d", rr.Code)
	}
}

func TestAppPreviewEnvForeignOrgAppIs404(t *testing.T) {
	h := newPreviewEnvHarness(t, previewHarnessOpts{})
	h.installAppsPlugin(t)

	// An app that exists but belongs to a different org must 404.
	otherOrg := createTestOrg(t, h.db)
	otherAgent := model.Agent{ID: uuid.New(), OrgID: &otherOrg.ID, TeamID: firstTeamID(t, h.db, otherOrg.ID), Name: "Other " + uuid.NewString(), Model: "test", Status: "active"}
	otherChan := model.Channel{ID: uuid.New(), OrgID: otherOrg.ID, Name: "other-" + uuid.NewString(), TeamID: otherAgent.TeamID, DefaultAgentID: otherAgent.ID}
	otherSheet := model.Sheet{ID: uuid.New(), OrgID: otherOrg.ID, ChannelID: otherChan.ID, Slug: "other-" + uuid.NewString()[:8], Name: "Other"}
	for _, seed := range []any{&otherAgent, &otherChan, &otherSheet} {
		if err := h.db.Create(seed).Error; err != nil {
			t.Fatalf("seed foreign org: %v", err)
		}
	}
	foreign, err := h.svc.CreateApp(context.Background(), apps.CreateAppParams{
		OrgID: otherOrg.ID, ChannelID: otherChan.ID, SheetID: otherSheet.ID, Name: "Foreign App",
	})
	if err != nil {
		t.Fatalf("create foreign app: %v", err)
	}
	t.Cleanup(func() { h.db.Delete(&model.App{}, "org_id = ?", otherOrg.ID) })

	if rr := h.fetch(t, h.secret, foreign.ID.String(), "3000"); rr.Code != 404 {
		t.Fatalf("foreign org app status = %d body=%s", rr.Code, rr.Body.String())
	}

	// Archived app of the caller's own org must also 404.
	if err := h.svc.ArchiveApp(context.Background(), h.org.ID, h.app.ID); err != nil {
		t.Fatalf("archive app: %v", err)
	}
	if rr := h.fetch(t, h.secret, h.app.ID.String(), "3000"); rr.Code != 404 {
		t.Fatalf("archived app status = %d body=%s", rr.Code, rr.Body.String())
	}
}

// TestAppPreviewEnvForeignAgentSameOrgIs404 proves preview-env is bound to the
// app's creating agent: a builder agent cannot fetch another team's app secret +
// team env by passing that app's id, even within the same org (H2 IDOR).
func TestAppPreviewEnvForeignAgentSameOrgIs404(t *testing.T) {
	h := newPreviewEnvHarness(t, previewHarnessOpts{})
	h.installAppsPlugin(t)

	// A second team's agent + channel + sheet in the SAME org, owning its own app.
	otherAgent := model.Agent{ID: uuid.New(), OrgID: &h.org.ID, TeamID: seedTeam(t, h.db, h.org.ID, "other-team").ID, Name: "Other Agent " + uuid.NewString(), Model: "test", Status: "active"}
	otherChan := model.Channel{ID: uuid.New(), OrgID: h.org.ID, Name: "other-ch-" + uuid.NewString(), TeamID: otherAgent.TeamID, DefaultAgentID: otherAgent.ID}
	otherSheet := model.Sheet{ID: uuid.New(), OrgID: h.org.ID, ChannelID: otherChan.ID, Slug: "other-" + uuid.NewString()[:8], Name: "Other"}
	for _, seed := range []any{&otherAgent, &otherChan, &otherSheet} {
		if err := h.db.Create(seed).Error; err != nil {
			t.Fatalf("seed other team: %v", err)
		}
	}
	t.Cleanup(func() {
		h.db.Delete(&model.Channel{}, "id = ?", otherChan.ID)
		h.db.Delete(&model.Agent{}, "id = ?", otherAgent.ID)
	})
	foreign, err := h.svc.CreateApp(context.Background(), apps.CreateAppParams{
		OrgID: h.org.ID, ChannelID: otherChan.ID, SheetID: otherSheet.ID, Name: "Foreign App",
		CreatedByAgentID: &otherAgent.ID,
	})
	if err != nil {
		t.Fatalf("create foreign app: %v", err)
	}

	// The harness agent's sandbox bearer is valid, but the app belongs to another
	// agent — it must be indistinguishable from a missing one.
	if rr := h.fetch(t, h.secret, foreign.ID.String(), "3000"); rr.Code != 404 {
		t.Fatalf("foreign-agent app status = %d body=%s, want 404", rr.Code, rr.Body.String())
	}

	// The caller's OWN app still resolves.
	if rr := h.fetch(t, h.secret, h.app.ID.String(), "3000"); rr.Code != 200 {
		t.Fatalf("own app status = %d body=%s, want 200", rr.Code, rr.Body.String())
	}
}

func TestAppPreviewEnvHappyPathMicrosandbox(t *testing.T) {
	h := newPreviewEnvHarness(t, previewHarnessOpts{})
	h.installAppsPlugin(t)

	// A team env var must ride along, decrypted.
	encValue, err := h.encKey.EncryptString("sk_live_preview")
	if err != nil {
		t.Fatalf("encrypt env var: %v", err)
	}
	envVar := model.TeamEnvVar{OrgID: h.org.ID, TeamID: h.channel.TeamID, Name: "STRIPE_KEY", EncryptedValue: encValue}
	if err := h.db.Create(&envVar).Error; err != nil {
		t.Fatalf("create team env var: %v", err)
	}
	t.Cleanup(func() { h.db.Delete(&model.TeamEnvVar{}, "id = ?", envVar.ID) })

	rr := h.fetch(t, h.secret, h.app.ID.String(), "3000")
	if rr.Code != 200 {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var resp previewEnvResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	// URL follows the microsandbox preview pattern with the base domain
	// derived from the builder sandbox's runtime URL.
	if want := "https://3000-msb-builder-1.preview.test.dev"; resp.PreviewURL != want {
		t.Fatalf("preview_url = %q, want %q", resp.PreviewURL, want)
	}

	// The env must be the exact deploy contract plus PORT.
	var stored model.App
	if err := h.db.First(&stored, "id = ?", h.app.ID).Error; err != nil {
		t.Fatalf("reload app: %v", err)
	}
	appSecret, err := h.encKey.DecryptString(stored.EncryptedAppSecret)
	if err != nil {
		t.Fatalf("decrypt app secret: %v", err)
	}
	sessionSecret, err := apps.DeriveSessionSecret(appSecret)
	if err != nil {
		t.Fatalf("derive session secret: %v", err)
	}
	want := map[string]string{
		"HIVY_APP_ID":          h.app.ID.String(),
		"HIVY_APP_SECRET":      appSecret,
		"HIVY_APP_API_URL":     "https://api.test/internal/apps/" + h.app.ID.String(),
		"HIVY_AUTH_PUBLIC_KEY": h.authPEM,
		"HIVY_LAUNCH_URL":      "https://web.test/",
		"HIVY_SESSION_SECRET":  sessionSecret,
		"HIVY_SHEET_ID":        h.sheet.ID.String(),
		"STRIPE_KEY":           "sk_live_preview",
		"PORT":                 "3000",
	}
	if len(resp.Env) != len(want) {
		t.Fatalf("env has %d keys, want %d: %v", len(resp.Env), len(want), envKeys(resp.Env))
	}
	for name, value := range want {
		if resp.Env[name] != value {
			t.Fatalf("env[%q] = %q, want %q", name, resp.Env[name], value)
		}
	}

	// The preview URL must be registered on the app row.
	if stored.PreviewURL != resp.PreviewURL {
		t.Fatalf("stored preview_url = %q, want %q", stored.PreviewURL, resp.PreviewURL)
	}
	if stored.PreviewRegisteredAt == nil {
		t.Fatal("preview_registered_at was not set")
	}
}

func TestAppPreviewEnvDockerProvider(t *testing.T) {
	h := newPreviewEnvHarness(t, previewHarnessOpts{
		providerID: sandbox.ProviderDocker,
		runtimeURL: "http://127.0.0.1:49000",
		cfg: &config.Config{
			FrontendURL:                "https://web.test",
			APIWebhookBaseURL:          "https://api.test",
			SandboxDockerControlOrigin: "http://host.docker.internal",
		},
	})
	h.installAppsPlugin(t)
	h.provider.endpoints[3000] = "http://127.0.0.1:49123"

	rr := h.fetch(t, h.secret, h.app.ID.String(), "3000")
	if rr.Code != 200 {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var resp previewEnvResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	// GetEndpoint's 127.0.0.1 host is rewritten to the docker control origin.
	if want := "http://host.docker.internal:49123"; resp.PreviewURL != want {
		t.Fatalf("preview_url = %q, want %q", resp.PreviewURL, want)
	}
	if resp.Env["PORT"] != "3000" {
		t.Fatalf("env PORT = %q", resp.Env["PORT"])
	}
}

func TestAppPreviewEnvDockerPortNotPublished(t *testing.T) {
	h := newPreviewEnvHarness(t, previewHarnessOpts{
		providerID: sandbox.ProviderDocker,
		runtimeURL: "http://127.0.0.1:49000",
	})
	h.installAppsPlugin(t)
	// Only 8080 resolves; the requested 5173 does not.
	h.provider.endpoints[8080] = "http://127.0.0.1:49222"

	rr := h.fetch(t, h.secret, h.app.ID.String(), "5173")
	if rr.Code != 400 {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, "5173") || !strings.Contains(body, "8080") {
		t.Fatalf("error must name the requested port and the published ports, got %s", body)
	}

	// No preview must be registered on failure.
	var stored model.App
	if err := h.db.First(&stored, "id = ?", h.app.ID).Error; err != nil {
		t.Fatalf("reload app: %v", err)
	}
	if stored.PreviewURL != "" || stored.PreviewRegisteredAt != nil {
		t.Fatalf("failed preview must not register: url=%q at=%v", stored.PreviewURL, stored.PreviewRegisteredAt)
	}
}

func envKeys(env map[string]string) []string {
	keys := make([]string, 0, len(env))
	for name := range env {
		keys = append(keys, name)
	}
	return keys
}
