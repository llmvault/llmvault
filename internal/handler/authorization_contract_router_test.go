package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/billing"
	billingfake "github.com/usehivy/hivy/internal/billing/fake"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
)

// buildAuthzRouter wires the exact /v1 MUTATION surfaces the authorization
// matrix probes, behind the SAME middleware tiers as cmd/server (the real
// setupV1Routes / mountBillingRoutes / mountOrgMemberLifecycleRoutes /
// setupConnectRoutes). As with buildContractRouter it omits only the identity
// front door (MultiAuth/RateLimit/Audit); the caller injects resolved org+claims
// via caller.apply, so every authorization gate that decides "may this principal
// perform this mutation" runs for real.
//
// Faithfulness note: agent Create/Update/Archive are member-reachable here
// BECAUSE the shipped router relaxed them out of the RequireOrgAdmin group
// (alongside triggers/schedules). The agent handlers enforce team-primary
// authorization themselves (resolveAndAuthorizeAgentTeam / authorizeAgentMutation):
// a member may CRUD an agent in a team they belong to, is denied (403) for a
// foreign team, and an unassigned (team_id IS NULL) agent stays manager-only.
// The suite asserts that shipped reality.
func buildAuthzRouter(db *gorm.DB) chi.Router {
	pluginHandler := handler.NewPluginHandler(db)
	teamProvHandler := handler.NewTeamProvisioningHandler(db)
	orgMemberHandler := handler.NewOrgMemberHandler(db)
	channelHandler := handler.NewChannelHandler(db)
	agentHandler := newAgentHandlerForTest(db)
	triggerHandler := handler.NewTriggerHandler(db)
	scheduleHandler := handler.NewScheduleHandler(db)
	sandboxHandler := handler.NewSandboxHandler(db, nil)
	connectionHandler := handler.NewConnectionHandler(db, nil, nil, nil)

	registry := billing.NewRegistry()
	registry.Register(billingfake.New("paystack"))
	credits := billing.NewCreditsService(db)
	billingHandler := handler.NewBillingHandler(db, registry, credits)
	subscriptionHandler := handler.NewSubscriptionHandler(db, registry, credits)

	r := chi.NewRouter()
	r.Route("/v1", func(r chi.Router) {
		// --- team provisioning + member lifecycle (mountOrgMemberLifecycleRoutes) --
		r.Route("/orgs/current", func(r chi.Router) {
			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireOrgAdmin(db))
				teamProvHandler.Mount(r)
				r.Patch("/members/{userID}/role", orgMemberHandler.PatchRole)
				r.Delete("/members/{userID}", orgMemberHandler.Remove)
			})
			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireOrgOwner(db))
				r.Post("/transfer-ownership", orgMemberHandler.TransferOwnership)
				r.Delete("/", orgMemberHandler.DeleteOrg)
			})
		})

		// --- billing (mountBillingRoutes): read admin+, money-moves owner-only ----
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireOrgAdmin(db))
			r.Get("/billing/subscription", billingHandler.GetSubscription)
		})
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireOrgOwner(db))
			r.Post("/billing/subscription/cancel", subscriptionHandler.Cancel)
		})

		// --- org catalog: plugin install/uninstall (admin-only) ------------------
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireOrgAdmin(db))
			r.Post("/plugins/{slug}/install", pluginHandler.Install)
			r.Delete("/plugins/{slug}/install", pluginHandler.Uninstall)
		})

		// --- connections (setupConnectRoutes): admin-only ------------------------
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireOrgAdmin(db))
			r.Post("/integrations/{id}/connections", connectionHandler.Create)
			r.Delete("/connections/{id}", connectionHandler.Revoke)
			r.Post("/connections/{id}/reconnect-session", connectionHandler.CreateReconnectSession)
		})

		// --- channels scope (member-reachable; handler enforces team) ------------
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireAPIKeyScopeOrJWT("channels"))
			r.Get("/channels", channelHandler.List)
			r.Get("/channels/{id}", channelHandler.Get)
			r.Get("/channels/{id}/agents", channelHandler.ListChannelAgents)
			r.Post("/channels/{id}/agents", channelHandler.AssignChannelAgent)
		})

		// --- agents scope --------------------------------------------------------
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireAPIKeyScopeOrJWT("agents"))
			r.Post("/agents/{id}/plugins/{slug}", pluginHandler.EnableForAgent)
			// Relaxed to team-members in the shipped router; the agent handlers
			// enforce team-primary authorization (resolveAndAuthorizeAgentTeam /
			// authorizeAgentMutation), so these routes are member-reachable and
			// NOT admin-gated.
			r.Post("/triggers", triggerHandler.Create)
			r.Post("/schedules", scheduleHandler.Create)
			r.Post("/agents", agentHandler.Create)
			r.Patch("/agents/{id}", agentHandler.Update)
			r.Delete("/agents/{id}", agentHandler.Archive)
			r.Post("/sandboxes/{id}/exec", sandboxHandler.Exec)
		})
	})
	return r
}

// authzReq issues a JSON request against router as caller cl in world w. It sets
// org context + JWT/api-key claims exactly as the auth middleware would.
func authzReq(router chi.Router, w authzWorld, cl caller, method, path string, body any) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	req = cl.apply(req, w.org)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	return rr
}
