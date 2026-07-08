package handler_test

import (
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	ragmodel "github.com/usehivy/hivy/internal/rag/model"
)

// newTeamProvHarness reuses the channel harness DB/auth but mounts the
// team-provisioning routes via Mount. Note: RequireOrgAdmin is applied by the
// route layer in production; these handler tests exercise the handler directly.
func newTeamProvHarness(t *testing.T) *channelHarness {
	t.Helper()
	h := newChannelHarness(t)
	tp := handler.NewTeamProvisioningHandler(h.db)
	r := chi.NewRouter()
	r.Route("/v1/orgs/current", func(r chi.Router) {
		r.Use(middleware.ResolveOrgFromHeader(h.db))
		r.Use(middleware.RequireAPIKeyScopeOrJWT("teams"))
		tp.MountReadable(r)
		tp.Mount(r)
	})
	h.router = r
	return h
}

func tpSeedTeam(t *testing.T, db *gorm.DB, orgID uuid.UUID) model.Team {
	t.Helper()
	team := model.Team{ID: uuid.New(), OrgID: orgID, Name: "tp-team-" + uuid.NewString()[:8]}
	if err := db.Create(&team).Error; err != nil {
		t.Fatalf("create team: %v", err)
	}
	return team
}

func tpSeedInstalledPlugin(t *testing.T, db *gorm.DB, orgID uuid.UUID) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if err := db.Create(&model.Plugin{ID: id, Slug: "tp-" + uuid.NewString()[:8], Name: "tp-plugin", Status: model.PluginStatusActive}).Error; err != nil {
		t.Fatalf("create plugin: %v", err)
	}
	if err := db.Create(&model.OrgPluginInstall{ID: uuid.New(), OrgID: orgID, PluginID: id}).Error; err != nil {
		t.Fatalf("install plugin: %v", err)
	}
	t.Cleanup(func() {
		db.Where("plugin_id = ?", id).Delete(&model.OrgPluginInstall{})
		db.Where("id = ?", id).Delete(&model.Plugin{})
	})
	return id
}

func tpSeedSource(t *testing.T, db *gorm.DB, orgID uuid.UUID) uuid.UUID {
	t.Helper()
	src := ragmodel.RAGSource{
		ID: uuid.New(), OrgIDValue: orgID, KindValue: ragmodel.RAGSourceKindWebsite,
		Name: "tp-src-" + uuid.NewString()[:8], Status: ragmodel.RAGSourceStatusActive, Enabled: true,
		ConfigValue: model.JSON{"url": "https://x.example"},
	}
	if err := db.Create(&src).Error; err != nil {
		t.Fatalf("create source: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", src.ID).Delete(&ragmodel.RAGSource{}) })
	return src.ID
}

type tpPluginsOut struct {
	Data []struct {
		ID   string `json:"id"`
		Slug string `json:"slug"`
		Name string `json:"name"`
	} `json:"data"`
}

type tpSourcesOut struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

func TestTeamProvisioning_PluginLifecycle(t *testing.T) {
	h := newTeamProvHarness(t)
	fx := (&channelHarness{db: h.db}).seed(t)
	team := tpSeedTeam(t, h.db, fx.org.ID)
	pluginID := tpSeedInstalledPlugin(t, h.db, fx.org.ID)
	base := "/v1/orgs/current/teams/" + team.ID.String() + "/plugins"

	// Empty to start.
	rr := h.doJSON(t, http.MethodGet, base, fx, fx.owner, nil)
	var listed tpPluginsOut
	decodeInto(t, rr, &listed)
	if rr.Code != http.StatusOK || len(listed.Data) != 0 {
		t.Fatalf("initial list: code=%d n=%d", rr.Code, len(listed.Data))
	}

	// Enable.
	rr = h.doJSON(t, http.MethodPost, base, fx, fx.owner, map[string]any{"plugin_id": pluginID.String()})
	if rr.Code != http.StatusCreated {
		t.Fatalf("enable: code=%d body=%s", rr.Code, rr.Body.String())
	}
	decodeInto(t, rr, &listed)
	if len(listed.Data) != 1 || listed.Data[0].ID != pluginID.String() {
		t.Fatalf("after enable = %+v", listed.Data)
	}

	// Re-enable → 409.
	rr = h.doJSON(t, http.MethodPost, base, fx, fx.owner, map[string]any{"plugin_id": pluginID.String()})
	if rr.Code != http.StatusConflict {
		t.Fatalf("re-enable: code=%d, want 409", rr.Code)
	}

	// Disable.
	rr = h.doJSON(t, http.MethodDelete, base+"/"+pluginID.String(), fx, fx.owner, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("disable: code=%d body=%s", rr.Code, rr.Body.String())
	}
	decodeInto(t, rr, &listed)
	if len(listed.Data) != 0 {
		t.Fatalf("after disable = %+v", listed.Data)
	}
}

func TestTeamProvisioning_PluginErrors(t *testing.T) {
	h := newTeamProvHarness(t)
	fx := (&channelHarness{db: h.db}).seed(t)
	team := tpSeedTeam(t, h.db, fx.org.ID)

	// Not-installed plugin → 422.
	notInstalled := uuid.New()
	if err := h.db.Create(&model.Plugin{ID: notInstalled, Slug: "ni-" + uuid.NewString()[:8], Name: "ni", Status: model.PluginStatusActive}).Error; err != nil {
		t.Fatalf("create plugin: %v", err)
	}
	t.Cleanup(func() { h.db.Where("id = ?", notInstalled).Delete(&model.Plugin{}) })
	base := "/v1/orgs/current/teams/" + team.ID.String() + "/plugins"
	rr := h.doJSON(t, http.MethodPost, base, fx, fx.owner, map[string]any{"plugin_id": notInstalled.String()})
	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("not-installed: code=%d, want 422 body=%s", rr.Code, rr.Body.String())
	}

	// Unknown plugin → 404.
	rr = h.doJSON(t, http.MethodPost, base, fx, fx.owner, map[string]any{"plugin_id": uuid.New().String()})
	if rr.Code != http.StatusNotFound {
		t.Fatalf("unknown plugin: code=%d, want 404", rr.Code)
	}
}

// Auto-install system plugins are always enabled and are not per-team
// provisionable: enabling one returns 422, and the team plugins list never
// includes it even when a stale team_plugins row exists.
func TestTeamProvisioning_AutoInstallExcluded(t *testing.T) {
	h := newTeamProvHarness(t)
	fx := (&channelHarness{db: h.db}).seed(t)
	team := tpSeedTeam(t, h.db, fx.org.ID)

	sysID := uuid.New()
	if err := h.db.Create(&model.Plugin{
		ID: sysID, Slug: "tp-sys-" + uuid.NewString()[:8], Name: "tp-sys",
		Status: model.PluginStatusActive, Manifest: model.RawJSON(`{"auto_install": true, "locked": true}`),
	}).Error; err != nil {
		t.Fatalf("create auto-install plugin: %v", err)
	}
	if err := h.db.Create(&model.OrgPluginInstall{ID: uuid.New(), OrgID: fx.org.ID, PluginID: sysID}).Error; err != nil {
		t.Fatalf("install auto-install plugin: %v", err)
	}
	t.Cleanup(func() {
		h.db.Where("plugin_id = ?", sysID).Delete(&model.OrgPluginInstall{})
		h.db.Where("plugin_id = ?", sysID).Delete(&model.TeamPlugin{})
		h.db.Where("id = ?", sysID).Delete(&model.Plugin{})
	})

	base := "/v1/orgs/current/teams/" + team.ID.String() + "/plugins"
	// Enable via API → 422 (always enabled, not toggleable).
	rr := h.doJSON(t, http.MethodPost, base, fx, fx.owner, map[string]any{"plugin_id": sysID.String()})
	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("enable auto-install: code=%d, want 422 body=%s", rr.Code, rr.Body.String())
	}

	// Even a stale/legacy team_plugins row must not surface in the list.
	if err := h.db.Create(&model.TeamPlugin{OrgID: fx.org.ID, TeamID: team.ID, PluginID: sysID}).Error; err != nil {
		t.Fatalf("seed stale team_plugins row: %v", err)
	}
	rr = h.doJSON(t, http.MethodGet, base, fx, fx.owner, nil)
	var listed tpPluginsOut
	decodeInto(t, rr, &listed)
	if rr.Code != http.StatusOK || len(listed.Data) != 0 {
		t.Fatalf("list excludes auto-install: code=%d data=%+v", rr.Code, listed.Data)
	}
}

// TestTeamProvisioning_OrgScoping: a caller in org A cannot provision org B's
// team, nor enable a plugin that belongs to another org.
func TestTeamProvisioning_OrgScoping(t *testing.T) {
	h := newTeamProvHarness(t)
	fx := (&channelHarness{db: h.db}).seed(t)    // org A
	other := (&channelHarness{db: h.db}).seed(t) // org B
	otherTeam := tpSeedTeam(t, h.db, other.org.ID)

	// Org A context, org B's team → 404 (team not in org).
	base := "/v1/orgs/current/teams/" + otherTeam.ID.String() + "/plugins"
	pluginA := tpSeedInstalledPlugin(t, h.db, fx.org.ID)
	rr := h.doJSON(t, http.MethodPost, base, fx, fx.owner, map[string]any{"plugin_id": pluginA.String()})
	if rr.Code != http.StatusNotFound {
		t.Fatalf("cross-org team: code=%d, want 404 body=%s", rr.Code, rr.Body.String())
	}

	// Plugin owned+installed by org B cannot be enabled from org A's team.
	teamA := tpSeedTeam(t, h.db, fx.org.ID)
	pluginB := uuid.New()
	if err := h.db.Create(&model.Plugin{ID: pluginB, OrgID: &other.org.ID, Slug: "b-" + uuid.NewString()[:8], Name: "b", Status: model.PluginStatusActive}).Error; err != nil {
		t.Fatalf("create org-B plugin: %v", err)
	}
	if err := h.db.Create(&model.OrgPluginInstall{ID: uuid.New(), OrgID: other.org.ID, PluginID: pluginB}).Error; err != nil {
		t.Fatalf("install org-B plugin: %v", err)
	}
	t.Cleanup(func() {
		h.db.Where("plugin_id = ?", pluginB).Delete(&model.OrgPluginInstall{})
		h.db.Where("id = ?", pluginB).Delete(&model.Plugin{})
	})
	baseA := "/v1/orgs/current/teams/" + teamA.ID.String() + "/plugins"
	rr = h.doJSON(t, http.MethodPost, baseA, fx, fx.owner, map[string]any{"plugin_id": pluginB.String()})
	if rr.Code != http.StatusNotFound {
		t.Fatalf("cross-org plugin: code=%d, want 404 body=%s", rr.Code, rr.Body.String())
	}
}

func TestTeamProvisioning_RagSourceLifecycle(t *testing.T) {
	h := newTeamProvHarness(t)
	fx := (&channelHarness{db: h.db}).seed(t)
	team := tpSeedTeam(t, h.db, fx.org.ID)
	srcID := tpSeedSource(t, h.db, fx.org.ID)
	base := "/v1/orgs/current/teams/" + team.ID.String() + "/rag-sources"

	// Grant.
	rr := h.doJSON(t, http.MethodPost, base, fx, fx.owner, map[string]any{"rag_source_id": srcID.String()})
	if rr.Code != http.StatusCreated {
		t.Fatalf("grant: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var listed tpSourcesOut
	decodeInto(t, rr, &listed)
	if len(listed.Data) != 1 || listed.Data[0].ID != srcID.String() {
		t.Fatalf("after grant = %+v", listed.Data)
	}

	// Re-grant → 409.
	rr = h.doJSON(t, http.MethodPost, base, fx, fx.owner, map[string]any{"rag_source_id": srcID.String()})
	if rr.Code != http.StatusConflict {
		t.Fatalf("re-grant: code=%d, want 409", rr.Code)
	}

	// Cross-org source → 404.
	other := (&channelHarness{db: h.db}).seed(t)
	foreign := tpSeedSource(t, h.db, other.org.ID)
	rr = h.doJSON(t, http.MethodPost, base, fx, fx.owner, map[string]any{"rag_source_id": foreign.String()})
	if rr.Code != http.StatusNotFound {
		t.Fatalf("cross-org source: code=%d, want 404 body=%s", rr.Code, rr.Body.String())
	}

	// Revoke.
	rr = h.doJSON(t, http.MethodDelete, base+"/"+srcID.String(), fx, fx.owner, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("revoke: code=%d body=%s", rr.Code, rr.Body.String())
	}
	decodeInto(t, rr, &listed)
	if len(listed.Data) != 0 {
		t.Fatalf("after revoke = %+v", listed.Data)
	}
}
