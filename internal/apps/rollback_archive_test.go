package apps

import (
	"context"
	"errors"
	"testing"
)

func TestRollbackAndEnvSync(t *testing.T) {
	h := newAppsTestHarness(t)
	ctx := context.Background()
	appd := newFakeAppd(t)
	h.provider.endpoints[appdPort] = appd.server.URL
	seedTeamEnvVar(t, h, "FLAG", "on")

	app := h.createApp(t, "Rollback Target")
	sourceKey, bundleKey, sourceSHA, bundleSHA := h.seedDriveObjects(t, []byte("s1"), []byte("b1"))
	first, err := h.svc.Publish(ctx, PublishParams{
		OrgID: h.org.ID, AppID: app.ID,
		SourceKey: sourceKey, BundleKey: bundleKey,
		SourceSHA256: sourceSHA, BundleSHA256: bundleSHA,
	})
	if err != nil {
		t.Fatalf("publish v1: %v", err)
	}
	source2, bundle2 := []byte("s2"), []byte("b2")
	h.store.put(sourceKey, source2)
	h.store.put(bundleKey, bundle2)
	if _, err := h.svc.Publish(ctx, PublishParams{
		OrgID: h.org.ID, AppID: app.ID,
		SourceKey: sourceKey, BundleKey: bundleKey,
		SourceSHA256: sha256Hex(source2), BundleSHA256: sha256Hex(bundle2),
	}); err != nil {
		t.Fatalf("publish v2: %v", err)
	}

	rolled, err := h.svc.Rollback(ctx, h.org.ID, app.ID, first.ID)
	if err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if rolled.ID != first.ID {
		t.Fatalf("rolled version = %s, want %s", rolled.ID, first.ID)
	}
	rollbacks := appd.recorded("/rollback")
	if len(rollbacks) != 1 || rollbacks[0].Body["sha256"] != first.BundleSHA256 {
		t.Fatalf("rollback calls = %+v", rollbacks)
	}
	reloaded, err := h.svc.GetApp(ctx, h.org.ID, app.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.ActiveVersionID == nil || *reloaded.ActiveVersionID != first.ID {
		t.Fatalf("active version after rollback = %v", reloaded.ActiveVersionID)
	}

	if err := h.svc.EnvSync(ctx, h.org.ID, app.ID); err != nil {
		t.Fatalf("env sync: %v", err)
	}
	pushes := appd.recorded("/env")
	if len(pushes) != 1 {
		t.Fatalf("env pushes = %d, want 1", len(pushes))
	}
	vars, ok := pushes[0].Body["vars"].(map[string]any)
	if !ok {
		t.Fatalf("env push body = %v", pushes[0].Body)
	}
	if vars["FLAG"] != "on" || vars["HIVY_APP_ID"] != app.ID.String() {
		t.Fatalf("env push vars = %v", vars)
	}
}

func TestArchiveStopsSandboxBestEffort(t *testing.T) {
	h := newAppsTestHarness(t)
	ctx := context.Background()
	appd := newFakeAppd(t)
	h.provider.endpoints[appdPort] = appd.server.URL

	app := h.createApp(t, "Archive Me")
	sourceKey, bundleKey, sourceSHA, bundleSHA := h.seedDriveObjects(t, []byte("s"), []byte("b"))
	if _, err := h.svc.Publish(ctx, PublishParams{
		OrgID: h.org.ID, AppID: app.ID,
		SourceKey: sourceKey, BundleKey: bundleKey,
		SourceSHA256: sourceSHA, BundleSHA256: bundleSHA,
	}); err != nil {
		t.Fatalf("publish: %v", err)
	}

	if err := h.svc.ArchiveApp(ctx, h.org.ID, app.ID); err != nil {
		t.Fatalf("archive: %v", err)
	}
	if len(h.provider.stopped) != 1 {
		t.Fatalf("stopped sandboxes = %v, want one", h.provider.stopped)
	}
	if _, err := h.svc.GetApp(ctx, h.org.ID, app.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("archived app lookup = %v, want ErrNotFound", err)
	}
}
