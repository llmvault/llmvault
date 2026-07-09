package handler_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
)

// buildContractRouter wires the exact /v1 read surfaces from the checklist behind
// the SAME authorization middleware as cmd/server/setupV1Routes
// (ResolveOrgFlexible / RequireAPIKeyScopeOrJWT / RequireOrgAdmin / ResolveUser).
//
// SHORTCUT (documented): it deliberately omits the token-parsing front door
// (MultiAuth / RequireEmailConfirmed / RateLimit / Audit). Those establish
// *identity* and are orthogonal to visibility; the caller injects the resolved
// org + user/api-key claims via caller.apply, exactly as MultiAuth would. Every
// gate that decides *what an identity may see* is exercised for real.
func buildContractRouter(db *gorm.DB) chi.Router {
	agentHandler := newAgentHandlerForTest(db)
	pluginHandler := handler.NewPluginHandler(db)
	triggerHandler := handler.NewTriggerHandler(db)
	scheduleHandler := handler.NewScheduleHandler(db)
	deliveryHandler := handler.NewTriggerDeliveryHandler(db)
	sandboxHandler := handler.NewSandboxHandler(db, nil)
	canvasHandler := newVisCanvasHandler(db)
	memoryHandler := handler.NewMemoryHandler(db, nil, nil, nil)
	directiveHandler := handler.NewDirectiveHandler(db)
	observationHandler := handler.NewObservationHandler(db)
	sessionHandler := handler.NewSessionHandler(db)
	channelHandler := handler.NewChannelHandler(db)
	ragSourceHandler := handler.NewRAGSourceHandler(db, nil, nil, nil, nil, nil)
	auditHandler := handler.NewAuditHandler(db)

	r := chi.NewRouter()
	r.Route("/v1", func(r chi.Router) {
		r.Use(middleware.ResolveOrgFlexible(db))

		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireOrgAdmin(db))
			r.Get("/audit", auditHandler.List)
		})

		r.Get("/canvas/projects", canvasHandler.ListProjects)
		r.Get("/canvas/artifacts", canvasHandler.ListArtifacts)
		r.Get("/canvas/artifacts/{artifactID}", canvasHandler.GetArtifact)
		r.Get("/plugins", pluginHandler.List)
		r.Get("/plugins/{slug}", pluginHandler.Get)

		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireAPIKeyScopeOrJWT("channels"))
			r.Get("/channels", channelHandler.List)
			r.Get("/channels/{id}", channelHandler.Get)
		})
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireAPIKeyScopeOrJWT("sessions"))
			r.Get("/sessions", sessionHandler.List)
			r.Get("/sessions/{id}", sessionHandler.Get)
		})
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireAPIKeyScopeOrJWT("memories"))
			r.Get("/memories", memoryHandler.List)
			r.Get("/memories/grouped", memoryHandler.Grouped)
			r.Get("/memories/channels/{channelId}", memoryHandler.ListChannel)
			r.Get("/directives", directiveHandler.List)
			r.Get("/observations", observationHandler.List)
		})
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireAPIKeyScopeOrJWT("agents"))
			r.Get("/agents", agentHandler.List)
			r.Get("/agents/catalog", agentHandler.ListCatalog)
			r.Get("/agents/{id}", agentHandler.Get)
			r.Get("/agents/{id}/trigger-deliveries", deliveryHandler.List)
			r.Get("/triggers", triggerHandler.List)
			r.Get("/triggers/{id}", triggerHandler.Get)
			r.Get("/schedules", scheduleHandler.List)
			r.Get("/schedules/{id}", scheduleHandler.Get)
			r.Route("/sandboxes", func(r chi.Router) {
				r.Get("/", sandboxHandler.List)
				r.Get("/{id}", sandboxHandler.Get)
			})
		})
		r.Route("/rag", func(r chi.Router) {
			r.Use(middleware.ResolveUser(db))
			r.Get("/sources", ragSourceHandler.List)
			r.Get("/sources/{id}/channels", ragSourceHandler.GetSourceChannels)
		})
	})
	return r
}

type contractCase struct {
	name         string
	path         string
	memberStatus int    // expected status for member M
	wantMember   string // token member SHOULD see (anti-vacuous); "" to skip
	adminStatus  int    // expected status for admin A0
	wantAdmin    string // B-world token admin SHOULD see (gate not over-filtering)
}

// TestVisibilityContract is the permanent cross-cutting guard: for one org it
// proves a non-manager member M (on teamA only) can never see teamB's channels,
// agents, or any resource scoped to them, across every read surface — while an
// admin still sees everything (the gate does not over-filter).
func TestVisibilityContract(t *testing.T) {
	db := connectTestDB(t)
	w := seedContractWorld(t, db)
	router := buildContractRouter(db)

	B := w.base // shorthand for the base fixture ids

	cases := []contractCase{
		// ---- agents / triggers / schedules / deliveries (scope "agents") ----
		{"agents.list", "/v1/agents", 200, B.visibleAgent.ID.String(), 200, B.hiddenAgent.ID.String()},
		{"agents.get.hidden", "/v1/agents/" + B.hiddenAgent.ID.String(), 404, "", 200, w.agentBInstr},
		{"agents.catalog", "/v1/agents/catalog", 200, w.catalogSlug, 200, B.hiddenAgent.ID.String()},
		{"agents.deliveries.hidden", "/v1/agents/" + B.hiddenAgent.ID.String() + "/trigger-deliveries", 404, "", 200, w.deliveryBMark},
		{"triggers.list", "/v1/triggers", 200, w.triggerA.String(), 200, w.triggerB.String()},
		{"triggers.get.hidden", "/v1/triggers/" + w.triggerB.String(), 404, "", 200, w.triggerBInstr},
		{"schedules.list", "/v1/schedules", 200, w.schedATask, 200, w.schedBTask},
		{"schedules.get.hidden", "/v1/schedules/" + w.schedB.String(), 404, "", 200, w.schedBTask},
		{"sandboxes.list", "/v1/sandboxes", 200, w.sandboxA.String(), 200, w.sandboxB.String()},
		{"sandboxes.get.hidden", "/v1/sandboxes/" + w.sandboxB.String(), 404, "", 200, w.sandboxB.String()},

		// ---- memories / observations / directives (scope "memories") ----
		{"memories.list", "/v1/memories", 200, w.memA, 200, w.memB},
		{"memories.grouped", "/v1/memories/grouped", 200, w.memA, 200, w.memB},
		{"memories.channel.hidden", "/v1/memories/channels/" + B.hiddenCh.ID.String(), 403, "", 200, w.memB},
		{"observations.list", "/v1/observations", 200, w.obsA, 200, w.obsB},
		{"directives.list", "/v1/directives", 200, w.dirA, 200, w.dirB},

		// ---- plugins (member-visible, agent-scoped enabled ids) ----
		{"plugins.list", "/v1/plugins", 200, w.pluginSlug, 200, B.hiddenAgent.ID.String()},
		{"plugins.get", "/v1/plugins/" + w.pluginSlug, 200, w.pluginSlug, 200, B.hiddenAgent.ID.String()},

		// ---- sandboxes handled above; canvas below ----
		{"canvas.projects", "/v1/canvas/projects", 200, w.projA.String(), 200, w.projB.String()},
		{"canvas.artifacts", "/v1/canvas/artifacts", 200, w.artifactA.String(), 200, w.artifactB.String()},
		{"canvas.artifact.hidden", "/v1/canvas/artifacts/" + w.artifactB.String(), 404, "", 200, w.artifactB.String()},

		// ---- rag (scope: ResolveUser; sources are org-wide, grants are scoped) ----
		{"rag.sources", "/v1/rag/sources", 200, w.srcAName, 200, w.srcAName},
		{"rag.source.channels.visible", "/v1/rag/sources/" + w.srcA.String() + "/channels", 200, B.visibleCh.ID.String(), 200, B.visibleCh.ID.String()},
		{"rag.source.channels.hidden", "/v1/rag/sources/" + w.srcB.String() + "/channels", 200, "", 200, B.hiddenCh.ID.String()},

		// ---- sessions (scope "sessions") ----
		{"sessions.list", "/v1/sessions", 200, w.sessOwnedA.String(), 200, B.hiddenSess.String()},
		{"sessions.get.hidden", "/v1/sessions/" + B.hiddenSess.String(), 403, "", 200, B.hiddenSess.String()},

		// ---- channels (scope "channels") ----
		{"channels.list", "/v1/channels", 200, B.visibleCh.ID.String(), 200, B.hiddenCh.ID.String()},
		{"channels.get.hidden", "/v1/channels/" + B.hiddenCh.ID.String(), 403, "", 200, B.hiddenCh.ID.String()},

		// ---- audit (admin-only) ----
		{"audit", "/v1/audit", 403, "", 200, ""},
	}

	// NOTE: POST /v1/rag/search is intentionally NOT in this table. Its scoping
	// filters vector hits and requires a live Qdrant backend, which is not
	// available in the handler test env; its channel scoping is covered by the
	// dedicated white-box test in the rag search package. Skipped, not faked.
	t.Log("skipping POST /v1/rag/search in HTTP contract table: needs Qdrant; covered by rag search white-box test")

	do := func(t *testing.T, c contractCase, cl caller) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, c.path, nil)
		req = cl.apply(req, w.base.org)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		return rr
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// Member M: correct status, ZERO forbidden tokens, and (anti-vacuous)
			// the A-world token where one is expected.
			mrr := do(t, c, memberCaller(w.base))
			if mrr.Code != c.memberStatus {
				t.Fatalf("member %s: status=%d want=%d body=%s", c.path, mrr.Code, c.memberStatus, mrr.Body.String())
			}
			body := mrr.Body.String()
			for _, tok := range w.forbidden {
				if strings.Contains(body, tok) {
					t.Fatalf("LEAK: member %s response contains forbidden token %q\nbody=%s", c.path, tok, body)
				}
			}
			if c.wantMember != "" && !strings.Contains(body, c.wantMember) {
				t.Fatalf("member %s: response missing expected token %q (test would be vacuous)\nbody=%s", c.path, c.wantMember, body)
			}

			// Admin A0: the gate must not over-filter — the B-world resource is visible.
			arr := do(t, c, adminCaller(w.base))
			if arr.Code != c.adminStatus {
				t.Fatalf("admin %s: status=%d want=%d body=%s", c.path, arr.Code, c.adminStatus, arr.Body.String())
			}
			if c.wantAdmin != "" && !strings.Contains(arr.Body.String(), c.wantAdmin) {
				t.Fatalf("admin %s: response missing B-world token %q (gate over-filters)\nbody=%s", c.path, c.wantAdmin, arr.Body.String())
			}
		})
	}
}
