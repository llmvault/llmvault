package apps

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/usehivy/hivy/internal/model"
)

// aliasStubProvider adds AliasProvider support on top of the base stub so the
// deploy path claims/releases aliases (the base stubProvider does NOT implement
// AliasProvider, exercising the silent-skip path for docker/local).
type aliasStubProvider struct {
	*stubProvider

	amu      sync.Mutex
	claims   []aliasClaimRecord
	released []string
	claimErr error
}

type aliasClaimRecord struct {
	alias     string
	sandboxID string
	port      int
}

func (p *aliasStubProvider) ClaimAlias(_ context.Context, alias, sandboxExternalID string, port int) (string, error) {
	p.amu.Lock()
	defer p.amu.Unlock()
	if p.claimErr != nil {
		return "", p.claimErr
	}
	p.claims = append(p.claims, aliasClaimRecord{alias: alias, sandboxID: sandboxExternalID, port: port})
	return "https://" + alias + ".preview.test", nil
}

func (p *aliasStubProvider) ReleaseAlias(_ context.Context, alias string) error {
	p.amu.Lock()
	defer p.amu.Unlock()
	p.released = append(p.released, alias)
	return nil
}

func (p *aliasStubProvider) claimCount() int {
	p.amu.Lock()
	defer p.amu.Unlock()
	return len(p.claims)
}

// installAliasProvider swaps the harness service onto an alias-capable provider
// wrapping the harness's base stub (same endpoints, create/stop bookkeeping).
func installAliasProvider(h *appsTestHarness) *aliasStubProvider {
	prov := &aliasStubProvider{stubProvider: h.provider}
	h.svc.provider = prov
	return prov
}

func publishOnce(t *testing.T, h *appsTestHarness, app *model.App) *model.AppVersion {
	t.Helper()
	sourceKey, bundleKey, sourceSHA, bundleSHA := h.seedDriveObjects(t, []byte("s"), []byte("b"))
	version, err := h.svc.Publish(context.Background(), PublishParams{
		OrgID: h.org.ID, AppID: app.ID,
		SourceKey: sourceKey, BundleKey: bundleKey,
		SourceSHA256: sourceSHA, BundleSHA256: bundleSHA,
	})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	return version
}

func TestDeployClaimsAlias(t *testing.T) {
	h := newAppsTestHarness(t)
	ctx := context.Background()
	appd := newFakeAppd(t)
	h.provider.endpoints[appdPort] = appd.server.URL
	h.provider.endpoints[appPort] = "http://127.0.0.1:45678"
	prov := installAliasProvider(h)

	app := h.createApp(t, "Alias App")
	if app.Alias != "alias-app" {
		t.Fatalf("alias stem = %q, want alias-app", app.Alias)
	}
	publishOnce(t, h, app)

	if len(prov.claims) != 1 {
		t.Fatalf("alias claims = %d, want 1", len(prov.claims))
	}
	claim := prov.claims[0]
	if claim.alias != "alias-app" {
		t.Fatalf("claimed alias = %q, want alias-app", claim.alias)
	}
	if claim.sandboxID != "stub-ext-1" {
		t.Fatalf("claimed sandbox = %q, want stub-ext-1", claim.sandboxID)
	}
	if claim.port != appPort {
		t.Fatalf("claimed port = %d, want %d", claim.port, appPort)
	}

	reloaded, err := h.svc.GetApp(ctx, h.org.ID, app.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.AliasURL != "https://alias-app.preview.test" {
		t.Fatalf("alias_url = %q", reloaded.AliasURL)
	}
	// AppURL prefers the alias URL over the sandbox endpoint.
	url, err := h.svc.AppURL(ctx, reloaded)
	if err != nil {
		t.Fatalf("app url: %v", err)
	}
	if url != "https://alias-app.preview.test" {
		t.Fatalf("AppURL = %q, want the alias url", url)
	}
}

func TestRedeployRepointsAlias(t *testing.T) {
	h := newAppsTestHarness(t)
	ctx := context.Background()
	appd := newFakeAppd(t)
	h.provider.endpoints[appdPort] = appd.server.URL
	prov := installAliasProvider(h)

	app := h.createApp(t, "Repoint App")
	version := publishOnce(t, h, app)

	// Simulate the app's sandbox being lost so redeploy provisions a fresh one.
	if err := h.db.Model(&model.App{}).Where("id = ?", app.ID).Update("sandbox_id", nil).Error; err != nil {
		t.Fatalf("clear sandbox link: %v", err)
	}
	reloaded, err := h.svc.GetApp(ctx, h.org.ID, app.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if err := h.svc.Deploy(ctx, reloaded, version); err != nil {
		t.Fatalf("redeploy: %v", err)
	}

	if len(prov.claims) != 2 {
		t.Fatalf("alias claims = %d, want 2 (claim + repoint)", len(prov.claims))
	}
	first, second := prov.claims[0], prov.claims[1]
	if second.alias != first.alias {
		t.Fatalf("repoint alias = %q, want same stem %q", second.alias, first.alias)
	}
	if second.sandboxID == first.sandboxID {
		t.Fatalf("repoint should target a new sandbox, both = %q", second.sandboxID)
	}
	if second.sandboxID != "stub-ext-2" {
		t.Fatalf("repoint sandbox = %q, want stub-ext-2", second.sandboxID)
	}
}

func TestDeployFailsWhenClaimAliasErrors(t *testing.T) {
	h := newAppsTestHarness(t)
	ctx := context.Background()
	appd := newFakeAppd(t)
	h.provider.endpoints[appdPort] = appd.server.URL
	prov := installAliasProvider(h)
	prov.claimErr = errors.New("control plane unreachable")

	app := h.createApp(t, "Claim Fails")
	sourceKey, bundleKey, sourceSHA, bundleSHA := h.seedDriveObjects(t, []byte("s"), []byte("b"))
	_, err := h.svc.Publish(ctx, PublishParams{
		OrgID: h.org.ID, AppID: app.ID,
		SourceKey: sourceKey, BundleKey: bundleKey,
		SourceSHA256: sourceSHA, BundleSHA256: bundleSHA,
	})
	if err == nil || !strings.Contains(err.Error(), "claim app alias") {
		t.Fatalf("deploy error = %v, want claim-alias failure", err)
	}
	reloaded, err := h.svc.GetApp(ctx, h.org.ID, app.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Status != model.AppStatusFailed {
		t.Fatalf("status = %q, want failed", reloaded.Status)
	}
	if reloaded.AliasURL != "" {
		t.Fatalf("alias_url = %q, want empty on failed claim", reloaded.AliasURL)
	}
}

func TestNonSupportingProviderSkipsAlias(t *testing.T) {
	h := newAppsTestHarness(t)
	ctx := context.Background()
	appd := newFakeAppd(t)
	h.provider.endpoints[appdPort] = appd.server.URL
	h.provider.endpoints[appPort] = "http://127.0.0.1:45678"
	// No alias provider installed: the base stub does not implement AliasProvider.

	app := h.createApp(t, "Plain App")
	publishOnce(t, h, app)

	reloaded, err := h.svc.GetApp(ctx, h.org.ID, app.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.AliasURL != "" {
		t.Fatalf("alias_url = %q, want empty (provider has no alias support)", reloaded.AliasURL)
	}
	// AppURL falls back to the plain sandbox endpoint.
	url, err := h.svc.AppURL(ctx, reloaded)
	if err != nil {
		t.Fatalf("app url: %v", err)
	}
	if url != "http://127.0.0.1:45678" {
		t.Fatalf("AppURL = %q, want the sandbox endpoint", url)
	}
}

func TestArchiveReleasesAliasBestEffort(t *testing.T) {
	h := newAppsTestHarness(t)
	ctx := context.Background()
	appd := newFakeAppd(t)
	h.provider.endpoints[appdPort] = appd.server.URL
	prov := installAliasProvider(h)

	app := h.createApp(t, "Archive Alias")
	publishOnce(t, h, app)
	if prov.claimCount() != 1 {
		t.Fatalf("expected the deploy to claim the alias first")
	}

	if err := h.svc.ArchiveApp(ctx, h.org.ID, app.ID); err != nil {
		t.Fatalf("archive: %v", err)
	}
	if len(prov.released) != 1 || prov.released[0] != "archive-alias" {
		t.Fatalf("released aliases = %v, want [archive-alias]", prov.released)
	}
}
