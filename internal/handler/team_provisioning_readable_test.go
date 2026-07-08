package handler_test

import (
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

func newTeamProvReadableHarness(t *testing.T) *channelHarness {
	t.Helper()
	h := newChannelHarness(t)
	tp := handler.NewTeamProvisioningHandler(h.db)
	r := chi.NewRouter()
	r.Route("/v1/orgs/current", func(r chi.Router) {
		r.Use(middleware.ResolveOrgFromHeader(h.db))
		r.Use(middleware.RequireAPIKeyScopeOrJWT("teams"))
		tp.MountReadable(r)
	})
	h.router = r
	return h
}

func TestTeamProvisioning_ListPluginsMemberReadable(t *testing.T) {
	h := newTeamProvReadableHarness(t)
	fx := (&channelHarness{db: h.db}).seed(t)
	team := tpSeedTeam(t, h.db, fx.org.ID)
	if err := h.db.Create(&model.TeamMember{OrgID: fx.org.ID, TeamID: team.ID, UserID: fx.member.ID, Role: "member"}).Error; err != nil {
		t.Fatalf("add team member: %v", err)
	}
	pluginID := tpSeedInstalledPlugin(t, h.db, fx.org.ID)
	if err := h.db.Create(&model.TeamPlugin{OrgID: fx.org.ID, TeamID: team.ID, PluginID: pluginID}).Error; err != nil {
		t.Fatalf("enable plugin: %v", err)
	}
	t.Cleanup(func() { h.db.Where("team_id = ?", team.ID).Delete(&model.TeamPlugin{}) })
	base := "/v1/orgs/current/teams/" + team.ID.String() + "/plugins"

	rr := h.doJSON(t, http.MethodGet, base, fx, fx.member, nil)
	var listed tpPluginsOut
	decodeInto(t, rr, &listed)
	if rr.Code != http.StatusOK || len(listed.Data) != 1 || listed.Data[0].ID != pluginID.String() {
		t.Fatalf("team member list: code=%d data=%+v", rr.Code, listed.Data)
	}

	rr = h.doJSON(t, http.MethodGet, base, fx, fx.owner, nil)
	decodeInto(t, rr, &listed)
	if rr.Code != http.StatusOK || len(listed.Data) != 1 {
		t.Fatalf("manager list: code=%d data=%+v", rr.Code, listed.Data)
	}

	stranger := h.seedUser(t, fx.org.ID, "member")
	t.Cleanup(func() {
		h.db.Where("user_id = ?", stranger.ID).Delete(&model.OrgMembership{})
		h.db.Where("id = ?", stranger.ID).Delete(&model.User{})
	})
	rr = h.doJSON(t, http.MethodGet, base, fx, stranger, nil)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("non-member list: code=%d, want 404 body=%s", rr.Code, rr.Body.String())
	}
}
