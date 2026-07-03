package apps

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/usehivy/hivy/internal/model"
)

// TestPublishFlow drives the full publish pipeline against the fake appd.
func TestPublishFlow(t *testing.T) {
	h := newAppsTestHarness(t)
	ctx := context.Background()
	appd := newFakeAppd(t)
	h.provider.endpoints[appdPort] = appd.server.URL
	h.provider.endpoints[appPort] = "http://127.0.0.1:45678"
	seedChannelEnvVar(t, h, "STRIPE_API_KEY", "sk_test_123")

	app := h.createApp(t, "Publish Flow")
	source := []byte("source-zip-bytes")
	bundle := []byte("bundle-zip-bytes")
	sourceKey, bundleKey, sourceSHA, bundleSHA := h.seedDriveObjects(t, source, bundle)

	version, err := h.svc.Publish(ctx, PublishParams{
		OrgID:           h.org.ID,
		AppID:           app.ID,
		SourceKey:       sourceKey,
		BundleKey:       bundleKey,
		SourceSHA256:    sourceSHA,
		BundleSHA256:    bundleSHA,
		Notes:           "first release",
		TemplateVersion: "tmpl-1",
		ActorAgentID:    &h.agent.ID,
	})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}

	// Drive keys went through the sheets admission rule.
	if len(h.keys.keys) != 2 || h.keys.keys[0] != sourceKey || h.keys.keys[1] != bundleKey {
		t.Fatalf("authorized keys = %v", h.keys.keys)
	}

	// Immutable org layout with content intact.
	wantSourceDst := fmt.Sprintf("pub/o/%s/apps/%s/%s/source.zip", h.org.ID, app.Slug, bundleSHA)
	wantBundleDst := fmt.Sprintf("pub/o/%s/apps/%s/%s/bundle.zip", h.org.ID, app.Slug, bundleSHA)
	if data, ok := h.store.get(wantSourceDst); !ok || string(data) != string(source) {
		t.Fatalf("source copy missing/mismatched at %s", wantSourceDst)
	}
	if data, ok := h.store.get(wantBundleDst); !ok || string(data) != string(bundle) {
		t.Fatalf("bundle copy missing/mismatched at %s", wantBundleDst)
	}

	// Version row.
	if version.SourceObjectKey != wantSourceDst || version.BundleObjectKey != wantBundleDst {
		t.Fatalf("version keys = %q / %q", version.SourceObjectKey, version.BundleObjectKey)
	}
	if version.SourceSHA256 != sourceSHA || version.BundleSHA256 != bundleSHA {
		t.Fatalf("version shas = %q / %q", version.SourceSHA256, version.BundleSHA256)
	}
	if version.SourceBytes != int64(len(source)) || version.BundleBytes != int64(len(bundle)) {
		t.Fatalf("version bytes = %d / %d", version.SourceBytes, version.BundleBytes)
	}
	if version.Notes != "first release" {
		t.Fatalf("notes = %q", version.Notes)
	}

	// Sandbox created from the app image with env at creation.
	if len(h.provider.created) != 1 {
		t.Fatalf("provider create calls = %d, want 1", len(h.provider.created))
	}
	createOpts := h.provider.created[0]
	if createOpts.TemplateRef != AppImageRef(h.cfg) {
		t.Fatalf("TemplateRef = %q, want %q", createOpts.TemplateRef, AppImageRef(h.cfg))
	}
	secret, err := h.encKey.DecryptString(app.EncryptedAppSecret)
	if err != nil {
		t.Fatalf("decrypt secret: %v", err)
	}
	if createOpts.EnvVars["HIVY_APP_SECRET"] != secret {
		t.Fatal("creation env is missing the app secret (appd cannot authenticate without it)")
	}
	if createOpts.Labels["app_id"] != app.ID.String() {
		t.Fatalf("labels = %v", createOpts.Labels)
	}

	// appd deploy call shape.
	deploys := appd.recorded("/deploy")
	if len(deploys) != 1 {
		t.Fatalf("deploy calls = %d, want 1", len(deploys))
	}
	deploy := deploys[0]
	if deploy.Bearer != secret {
		t.Fatal("deploy bearer is not the app secret")
	}
	if deploy.Body["sha256"] != bundleSHA {
		t.Fatalf("deploy sha256 = %v", deploy.Body["sha256"])
	}
	if deploy.Body["version_id"] != version.ID.String() {
		t.Fatalf("deploy version_id = %v", deploy.Body["version_id"])
	}
	if deploy.Body["bundle_url"] != "https://s3.test/presigned/"+wantBundleDst {
		t.Fatalf("deploy bundle_url = %v", deploy.Body["bundle_url"])
	}
	env, ok := deploy.Body["env"].(map[string]any)
	if !ok {
		t.Fatalf("deploy env missing: %v", deploy.Body)
	}
	sessionSecret, err := DeriveSessionSecret(secret)
	if err != nil {
		t.Fatalf("derive session secret: %v", err)
	}
	wantEnv := map[string]string{
		"HIVY_APP_ID":         app.ID.String(),
		"HIVY_APP_SECRET":     secret,
		"HIVY_APP_API_URL":    "https://api.test/internal/apps/" + app.ID.String(),
		"HIVY_LAUNCH_URL":     "https://web.test/",
		"HIVY_SESSION_SECRET": sessionSecret,
		"HIVY_SHEET_ID":       h.sheet.ID.String(),
		"STRIPE_API_KEY":      "sk_test_123", // channel var, PLAIN name
	}
	for name, want := range wantEnv {
		if env[name] != want {
			t.Fatalf("env[%s] = %v, want %q", name, env[name], want)
		}
	}
	if env["HIVY_AUTH_PUBLIC_KEY"] == "" {
		t.Fatal("env missing HIVY_AUTH_PUBLIC_KEY")
	}

	// App flipped to running with the active version + sandbox linked.
	reloaded, err := h.svc.GetApp(ctx, h.org.ID, app.ID)
	if err != nil {
		t.Fatalf("reload app: %v", err)
	}
	if reloaded.Status != model.AppStatusRunning {
		t.Fatalf("status = %q, want running", reloaded.Status)
	}
	if reloaded.ActiveVersionID == nil || *reloaded.ActiveVersionID != version.ID {
		t.Fatalf("active_version_id = %v", reloaded.ActiveVersionID)
	}
	if reloaded.SandboxID == nil {
		t.Fatal("sandbox_id not set")
	}
	if reloaded.TemplateVersion != "tmpl-1" {
		t.Fatalf("template_version = %q", reloaded.TemplateVersion)
	}
}

func TestPublishRejectsSHAMismatch(t *testing.T) {
	h := newAppsTestHarness(t)
	ctx := context.Background()
	app := h.createApp(t, "Mismatch")
	source := []byte("source-bytes")
	bundle := []byte("bundle-bytes")
	sourceKey, bundleKey, _, bundleSHA := h.seedDriveObjects(t, source, bundle)

	wrongSHA := sha256Hex([]byte("something else"))
	_, err := h.svc.Publish(ctx, PublishParams{
		OrgID: h.org.ID, AppID: app.ID,
		SourceKey: sourceKey, BundleKey: bundleKey,
		SourceSHA256: wrongSHA, BundleSHA256: bundleSHA,
	})
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("mismatch error = %v, want ValidationError", err)
	}

	// The mismatched copy was deleted and no version row exists.
	dst := fmt.Sprintf("pub/o/%s/apps/%s/%s/source.zip", h.org.ID, app.Slug, bundleSHA)
	if _, ok := h.store.get(dst); ok {
		t.Fatal("mismatched object was left behind at the content-addressed key")
	}
	var count int64
	if err := h.db.Model(&model.AppVersion{}).Where("app_id = ?", app.ID).Count(&count).Error; err != nil {
		t.Fatalf("count versions: %v", err)
	}
	if count != 0 {
		t.Fatalf("version rows = %d, want 0", count)
	}
}

func TestDeployFailureMarksAppFailed(t *testing.T) {
	h := newAppsTestHarness(t)
	ctx := context.Background()
	appd := newFakeAppd(t)
	appd.statuses["/deploy"] = http.StatusUnprocessableEntity
	h.provider.endpoints[appdPort] = appd.server.URL

	app := h.createApp(t, "Broken Deploy")
	sourceKey, bundleKey, sourceSHA, bundleSHA := h.seedDriveObjects(t, []byte("s"), []byte("b"))

	_, err := h.svc.Publish(ctx, PublishParams{
		OrgID: h.org.ID, AppID: app.ID,
		SourceKey: sourceKey, BundleKey: bundleKey,
		SourceSHA256: sourceSHA, BundleSHA256: bundleSHA,
	})
	var appdErr *AppdError
	if !errors.As(err, &appdErr) || appdErr.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("deploy error = %v, want AppdError 422", err)
	}
	reloaded, err := h.svc.GetApp(ctx, h.org.ID, app.ID)
	if err != nil {
		t.Fatalf("reload app: %v", err)
	}
	if reloaded.Status != model.AppStatusFailed {
		t.Fatalf("status = %q, want failed", reloaded.Status)
	}
}
