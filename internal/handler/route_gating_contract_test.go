package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

// seedOwner adds an org owner to an existing fixture org and returns the user.
func seedOwner(t *testing.T, db *gorm.DB, orgID uuid.UUID) model.User {
	t.Helper()
	owner := model.User{ID: uuid.New(), Email: "owner-" + uuid.NewString()[:8] + "@t.com", Name: "owner"}
	rows := []any{&owner, &model.OrgMembership{UserID: owner.ID, OrgID: orgID, Role: "owner"}}
	for _, r := range rows {
		if err := db.Create(r).Error; err != nil {
			t.Fatalf("seed owner: %v", err)
		}
	}
	t.Cleanup(func() {
		db.Where("org_id = ? AND user_id = ?", orgID, owner.ID).Delete(&model.OrgMembership{})
		db.Where("id = ?", owner.ID).Delete(&model.User{})
	})
	return owner
}

func ownerCaller(u model.User) caller { return caller{user: &u} }

// TestRouteGating_MiddlewareTiers proves the middleware-level authorization
// tiers this build sets: billing money-moves are owner-only, connections /
// plugin-install are admin-only, and the api-keys / tokens inventory lists are
// admin (or scoped API key), never a bare member. Stub handlers stand in for the
// real ones so the gate — not the handler body — is what's under test.
func TestRouteGating_MiddlewareTiers(t *testing.T) {
	db := connectTestDB(t)
	fx := seedVisFixture(t, db)
	owner := seedOwner(t, db, fx.org.ID)

	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	r := chi.NewRouter()
	r.Route("/v1", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireOrgOwner(db))
			r.Post("/billing/checkout", ok)
		})
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireOrgAdmin(db))
			r.Get("/connections", ok)
			r.Post("/plugins/{slug}/install", ok)
		})
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireOrgAdminOrAPIKey(db))
			r.Get("/api-keys", ok)
			r.Get("/tokens", ok)
		})
	})

	do := func(method, path string, c caller) int {
		req := httptest.NewRequest(method, path, nil)
		req = c.apply(req, fx.org)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		return rr.Code
	}

	type tc struct {
		name, method, path string
		owner, admin, mem  int
		apiKey             int // -1 = skip
	}
	cases := []tc{
		{"billing.checkout", "POST", "/v1/billing/checkout", 200, 403, 403, -1},
		{"connections.list", "GET", "/v1/connections", 200, 200, 403, -1},
		{"plugins.install", "POST", "/v1/plugins/x/install", 200, 200, 403, -1},
		{"apikeys.list", "GET", "/v1/api-keys", 200, 200, 403, 200},
		{"tokens.list", "GET", "/v1/tokens", 200, 200, 403, 200},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := do(c.method, c.path, ownerCaller(owner)); got != c.owner {
				t.Fatalf("owner %s: got %d want %d", c.path, got, c.owner)
			}
			if got := do(c.method, c.path, adminCaller(fx)); got != c.admin {
				t.Fatalf("admin %s: got %d want %d", c.path, got, c.admin)
			}
			if got := do(c.method, c.path, memberCaller(fx)); got != c.mem {
				t.Fatalf("member %s: got %d want %d", c.path, got, c.mem)
			}
			if c.apiKey >= 0 {
				if got := do(c.method, c.path, apiKeyCaller()); got != c.apiKey {
					t.Fatalf("apiKey %s: got %d want %d", c.path, got, c.apiKey)
				}
			}
		})
	}
}

// TestRouteGating_ChannelCreateTeamMembership proves POST /v1/channels enforces
// team membership: a non-manager may create a channel only inside a team they
// belong to, never in another team or a team-less channel; managers/API keys are
// unrestricted.
func TestRouteGating_ChannelCreateTeamMembership(t *testing.T) {
	db := connectTestDB(t)
	fx := seedVisFixture(t, db)
	h := handler.NewChannelHandler(db)

	// A team-less default agent is valid for a channel in ANY team (the cross-team
	// default rule only applies when both the channel and agent are team-scoped),
	// so it isolates this test to the team-membership gate under test.
	defAgent := model.Agent{ID: uuid.New(), OrgID: &fx.org.ID, Name: "rg-def-" + uuid.NewString()[:8], Model: "test", Status: "active"}
	if err := db.Create(&defAgent).Error; err != nil {
		t.Fatalf("seed default agent: %v", err)
	}

	var created []uuid.UUID
	t.Cleanup(func() {
		db.Where("channel_id IN ?", created).Delete(&model.ChannelMember{})
		db.Where("id IN ?", created).Delete(&model.Channel{})
		db.Where("id = ?", defAgent.ID).Delete(&model.Agent{})
	})

	create := func(c caller, teamID *uuid.UUID) int {
		body := map[string]any{
			"name":             "rg-" + uuid.NewString()[:8],
			"category":         "general",
			"default_agent_id": defAgent.ID.String(),
		}
		if teamID != nil {
			body["team_id"] = teamID.String()
		}
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/v1/channels", bytes.NewReader(raw))
		req = c.apply(req, fx.org)
		rr := httptest.NewRecorder()
		h.Create(rr, req)
		if rr.Code == http.StatusCreated {
			var resp struct {
				Channel struct {
					ID string `json:"id"`
				} `json:"channel"`
			}
			if err := json.Unmarshal(rr.Body.Bytes(), &resp); err == nil {
				if id, err := uuid.Parse(resp.Channel.ID); err == nil {
					created = append(created, id)
				}
			}
		}
		return rr.Code
	}

	teamA := fx.visibleCh.TeamID // member is an active member
	teamB := fx.hiddenCh.TeamID  // member is NOT

	if got := create(memberCaller(fx), teamA); got != http.StatusCreated {
		t.Fatalf("member in own team: got %d want 201", got)
	}
	if got := create(memberCaller(fx), teamB); got != http.StatusForbidden {
		t.Fatalf("member in other team: got %d want 403", got)
	}
	if got := create(memberCaller(fx), nil); got != http.StatusForbidden {
		t.Fatalf("member team-less: got %d want 403", got)
	}
	if got := create(adminCaller(fx), teamB); got != http.StatusCreated {
		t.Fatalf("admin in any team: got %d want 201", got)
	}
	if got := create(apiKeyCaller(), nil); got != http.StatusCreated {
		t.Fatalf("api key team-less: got %d want 201", got)
	}
}

// TestRouteGating_SandboxExecVisibleAgentOnly proves POST /v1/sandboxes/{id}/exec
// is unreachable for a sandbox whose agent the caller cannot see: a member gets
// 404 for a hidden-agent sandbox (indistinguishable from nonexistent) but passes
// the gate for a visible one (503 here, orchestrator not configured). Managers
// reach any sandbox.
func TestRouteGating_SandboxExecVisibleAgentOnly(t *testing.T) {
	db := connectTestDB(t)
	fx := seedVisFixture(t, db)
	h := handler.NewSandboxHandler(db, nil)
	visSb := seedVisSandbox(t, db, fx.org.ID, &fx.visibleAgent.ID, "rg-vis-"+uuid.NewString()[:6])
	hidSb := seedVisSandbox(t, db, fx.org.ID, &fx.hiddenAgent.ID, "rg-hid-"+uuid.NewString()[:6])

	exec := func(c caller, id uuid.UUID) int {
		raw, _ := json.Marshal(map[string]any{"commands": []string{"echo hi"}})
		req := httptest.NewRequest(http.MethodPost, "/v1/sandboxes/"+id.String()+"/exec", bytes.NewReader(raw))
		req = c.apply(req, fx.org)
		req = withURLParam(req, "id", id.String())
		rr := httptest.NewRecorder()
		h.Exec(rr, req)
		return rr.Code
	}

	if got := exec(memberCaller(fx), hidSb.ID); got != http.StatusNotFound {
		t.Fatalf("member exec hidden sandbox: got %d want 404", got)
	}
	if got := exec(memberCaller(fx), visSb.ID); got != http.StatusServiceUnavailable {
		t.Fatalf("member exec visible sandbox: got %d want 503 (past gate)", got)
	}
	if got := exec(adminCaller(fx), hidSb.ID); got != http.StatusServiceUnavailable {
		t.Fatalf("admin exec hidden sandbox: got %d want 503 (past gate)", got)
	}
}

// TestRouteGating_ScheduleCreateTeamManage proves the trigger/schedule create
// relaxation is guarded: creating a schedule bound to a channel is a manage-the-
// channel's-team action, not merely use-the-channel. A member who may USE a
// team-less channel (canUseChannel is open for it) is still denied (403) at the
// binding-manage gate, while a manager passes it. This is the exact escalation
// the relaxation must not open.
func TestRouteGating_ScheduleCreateTeamManage(t *testing.T) {
	db := connectTestDB(t)
	fx := seedVisFixture(t, db)
	h := handler.NewScheduleHandler(db)

	// A team-less channel: any member may use it (canUseChannel true), and any org
	// agent may act in it (team-less channels are unbounded), so the binding-manage
	// gate is what decides.
	ch := model.Channel{ID: uuid.New(), OrgID: fx.org.ID, Name: "teamless-" + uuid.NewString()[:8], Kind: "standard", DefaultAgentID: fx.visibleAgent.ID}
	if err := db.Create(&ch).Error; err != nil {
		t.Fatalf("seed team-less channel: %v", err)
	}
	t.Cleanup(func() {
		db.Where("org_id = ?", fx.org.ID).Delete(&model.AgentSchedule{})
		db.Where("id = ?", ch.ID).Delete(&model.Channel{})
	})

	interval := int64(3600)
	create := func(c caller) int {
		body := map[string]any{
			"name":             "sched-" + uuid.NewString()[:6],
			"agent_id":         fx.visibleAgent.ID.String(),
			"channel_id":       ch.ID.String(),
			"task_prompt":      "do a thing",
			"interval_seconds": interval,
		}
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/v1/schedules", bytes.NewReader(raw))
		req = c.apply(req, fx.org)
		rr := httptest.NewRecorder()
		h.Create(rr, req)
		return rr.Code
	}

	if got := create(memberCaller(fx)); got != http.StatusForbidden {
		t.Fatalf("member schedule on team-less channel: got %d want 403 (manage gate)", got)
	}
	if got := create(adminCaller(fx)); got == http.StatusForbidden {
		t.Fatalf("admin schedule on team-less channel: got 403, must pass the manage gate")
	}
}

// TestRouteGating_ReconnectSessionCrossOrgIDOR proves CreateReconnectSession is
// org-scoped: a connection id belonging to another org resolves to 404, closing
// the cross-org IDOR (no reconnect session minted for a foreign connection).
func TestRouteGating_ReconnectSessionCrossOrgIDOR(t *testing.T) {
	db := connectTestDB(t)
	fx := seedVisFixture(t, db)
	admin := fx.admin

	otherOrg := model.Org{ID: uuid.New(), Name: "other-" + uuid.NewString()[:8], RateLimit: 1000, Active: true}
	otherUser := model.User{ID: uuid.New(), Email: "ou-" + uuid.NewString()[:8] + "@t.com", Name: "ou"}
	integ := model.Integration{ID: uuid.New(), UniqueKey: "uk-" + uuid.NewString()[:8], Provider: "github", DisplayName: "GitHub"}
	conn := model.Connection{ID: uuid.New(), OrgID: otherOrg.ID, UserID: otherUser.ID, IntegrationID: integ.ID, NangoConnectionID: "ncid"}
	for _, row := range []any{&otherOrg, &otherUser, &integ, &conn} {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("seed other-org connection: %v", err)
		}
	}
	t.Cleanup(func() {
		db.Where("id = ?", conn.ID).Delete(&model.Connection{})
		db.Where("id = ?", integ.ID).Delete(&model.Integration{})
		db.Where("id = ?", otherUser.ID).Delete(&model.User{})
		db.Where("id = ?", otherOrg.ID).Delete(&model.Org{})
	})

	h := handler.NewConnectionHandler(db, nil, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/v1/connections/"+conn.ID.String()+"/reconnect-session", nil)
	req = middleware.WithOrg(req, &fx.org)
	req = middleware.WithUser(req, &admin)
	req = withURLParam(req, "id", conn.ID.String())
	rr := httptest.NewRecorder()
	h.CreateReconnectSession(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("cross-org reconnect: got %d want 404 body=%s", rr.Code, rr.Body.String())
	}
}
