package main

import (
	"crypto/rsa"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/cache"
	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/sandbox"
)

func setupV1Routes(
	r chi.Router,
	cfg *config.Config,
	rsaPub *rsa.PublicKey,
	database *gorm.DB,
	apiKeyCache *cache.APIKeyCache,
	enqueuer enqueue.TaskEnqueuer,
	orgHandler *handler.OrgHandler,
	orgInviteHandler *handler.OrgInviteHandler,
	brandHandler *handler.BrandHandler,
	teamHandler *handler.TeamHandler,
	usageHandler *handler.UsageHandler,
	auditHandler *handler.AuditHandler,
	reportingHandler *handler.ReportingHandler,
	generationHandler *handler.GenerationHandler,
	apiKeyHandler *handler.APIKeyHandler,
	billingHandler *handler.BillingHandler,
	subscriptionHandler *handler.SubscriptionHandler,
	dashboardHandler *handler.DashboardHandler,
	slackChannelHandler *handler.SlackChannelHandler,
	channelHandler *handler.ChannelHandler,
	sessionHandler *handler.SessionHandler,
	memoryHandler *handler.MemoryHandler,
	credHandler *handler.CredentialHandler,
	tokenHandler *handler.TokenHandler,
	sandboxTemplateHandler *handler.SandboxTemplateHandler,
	pluginHandler *handler.PluginHandler,
	databaseIntegrationHandler *handler.DatabaseIntegrationHandler,
	ragSourceHandler *handler.RAGSourceHandler,
	ragSearchHandler *handler.RAGSearchHandler,
	uploadsHandler *handler.UploadsHandler,
	imageDescribeHandler *handler.ImageDescribeHandler,
	agentHandler *handler.AgentHandler,
	canvasHandler *handler.CanvasHandler,
	sheetsHandler *handler.SheetsHandler,
	appsHandler *handler.AppsHandler,
	transcriptionHandler *handler.TranscriptionHandler,
	mcpServerHandler *handler.MCPServerHandler,
	orchestrator *sandbox.Orchestrator,
	auditWriter *middleware.AuditWriter,
) {
	r.Route("/v1", func(r chi.Router) {
		r.Use(middleware.MultiAuth(rsaPub, cfg.AuthIssuer, cfg.AuthAudience, database, apiKeyCache, enqueuer))
		r.Use(middleware.RequireEmailConfirmed(database))

		r.Post("/orgs", orgHandler.Create)
		// Authenticated invite accept/decline — user-scoped, no org context required.
		r.Post("/invites/{token}/accept", orgInviteHandler.Accept)
		r.Post("/invites/{token}/decline", orgInviteHandler.Decline)

		r.Group(func(r chi.Router) {
			r.Use(middleware.ResolveOrgFlexible(database))
			r.Use(middleware.RateLimit())
			r.Use(middleware.Audit(auditWriter))

			r.Get("/orgs/current", orgHandler.Current)
			if mcpServerHandler != nil {
				mcpServerHandler.Mount(r)
			}
			r.Get("/orgs/current/members", orgInviteHandler.ListMembers)
			mountBrandRoutes(r, database, brandHandler)

			// Reading teams is a member action (members pick a team when
			// creating agents/channels); the handlers scope results to the
			// caller, so these two routes are NOT admin-gated. Team write +
			// member management stay admin-only below.
			if teamHandler != nil {
				r.Get("/orgs/current/teams", teamHandler.List)
				r.Get("/orgs/current/teams/{id}", teamHandler.Get)
				r.Group(func(r chi.Router) {
					r.Use(middleware.RequireAPIKeyScopeOrJWT("teams"))
					r.Get("/orgs/current/teams/{id}/environment-variables", teamHandler.ListEnvironmentVariables)
					r.Post("/orgs/current/teams/{id}/environment-variables", teamHandler.CreateEnvironmentVariable)
					r.Patch("/orgs/current/teams/{id}/environment-variables/{name}", teamHandler.UpdateEnvironmentVariable)
					r.Delete("/orgs/current/teams/{id}/environment-variables/{name}", teamHandler.DeleteEnvironmentVariable)
				})
			}

			// Admin-only org invite management.
			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireOrgAdmin(database))
				r.Patch("/orgs/current", orgHandler.Update)
				r.Patch("/orgs/current/onboarding", orgHandler.AdvanceOnboarding)
				r.Post("/orgs/current/invites", orgInviteHandler.Create)
				r.Get("/orgs/current/invites", orgInviteHandler.List)
				r.Delete("/orgs/current/invites/{id}", orgInviteHandler.Revoke)
				r.Post("/orgs/current/invites/{id}/resend", orgInviteHandler.Resend)
				if teamHandler != nil {
					r.Post("/orgs/current/teams", teamHandler.Create)
					r.Patch("/orgs/current/teams/{id}", teamHandler.Update)
					r.Delete("/orgs/current/teams/{id}", teamHandler.Archive)
					r.Put("/orgs/current/teams/{id}/members/{userID}", teamHandler.PutMember)
					r.Delete("/orgs/current/teams/{id}/members/{userID}", teamHandler.DeleteMember)
				}

			})

			mountOrgMemberLifecycleRoutes(r, database)

			// Admin-only org-wide observability: audit, usage, generations,
			// dashboard, and reporting all expose org-wide request paths,
			// per-user cost/credit spend, client IP addresses, and user ids
			// that a non-admin member must not read.
			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireOrgAdmin(database))
				if dashboardHandler != nil {
					r.Get("/dashboard", dashboardHandler.Get)
				}
				r.Get("/audit", auditHandler.List)
				r.Get("/usage", usageHandler.Get)
				r.Get("/generations", generationHandler.List)
				r.Get("/generations/{id}", generationHandler.Get)
				r.Get("/reporting", reportingHandler.Get)
			})
			if canvasHandler != nil {
				r.Get("/canvas/projects", canvasHandler.ListProjects)
				r.Get("/canvas/artifacts", canvasHandler.ListArtifacts)
				r.Get("/canvas/artifacts/{artifactID}", canvasHandler.GetArtifact)
				r.Post("/canvas/artifacts/{artifactID}/preview-url", canvasHandler.PreviewArtifactURL)
			}
			if databaseIntegrationHandler != nil {
				r.Get("/database-integrations", databaseIntegrationHandler.List)
			}
			// Sheets are scope-gated for API keys like channels/agents; JWT
			// callers pass and channel-level access is enforced per sheet.
			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireAPIKeyScopeOrJWT("sheets"))
				mountSheetRoutes(r, database, sheetsHandler)
			})
			// Apps are channel-scoped like sheets; channel-level access is
			// enforced per app inside the handlers.
			mountAppRoutes(r, database, appsHandler)

			// API-key CREATE is org-admin-or-above only (owners+admins): a FINAL
			// maintainer decision. API-key callers may no longer mint keys — only a
			// human org admin may. RequireOrgAdmin rejects API-key auth (no JWT
			// claims), so keys are shut out of creation.
			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireOrgAdmin(database))
				r.Post("/api-keys", apiKeyHandler.Create)
			})
			// Read/revoke keep existing scoping: List leaks org-wide key inventory
			// so it stays admin-gated (API keys with scope still pass), Revoke too.
			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireOrgAdminOrAPIKey(database))
				r.Get("/api-keys", apiKeyHandler.List)
				r.Delete("/api-keys/{id}", apiKeyHandler.Revoke)
			})

			mountBillingRoutes(r, database, billingHandler, subscriptionHandler)
			if pluginHandler != nil {
				r.Get("/plugins", pluginHandler.List)
				r.Get("/plugins/{slug}", pluginHandler.Get)
				// Installing/uninstalling an org plugin is an admin-only mutation.
				r.Group(func(r chi.Router) {
					r.Use(middleware.RequireOrgAdmin(database))
					r.Post("/plugins/{slug}/install", pluginHandler.Install)
					r.Delete("/plugins/{slug}/install", pluginHandler.Uninstall)
				})
			}
			if slackChannelHandler != nil {
				r.Get("/slack/channels", slackChannelHandler.ListChannels)
				r.Post("/slack/channels/join", slackChannelHandler.JoinChannels)
			}
			if channelHandler != nil {
				r.Group(func(r chi.Router) {
					r.Use(middleware.RequireAPIKeyScopeOrJWT("channels"))
					r.Get("/channels", channelHandler.List)
					r.Post("/channels", channelHandler.Create)
					r.Get("/channels/{id}", channelHandler.Get)
					r.Patch("/channels/{id}", channelHandler.Update)
					r.Delete("/channels/{id}", channelHandler.Archive)
					r.Post("/channels/{id}/join", channelHandler.Join)
					r.Put("/channels/{id}/members/{userID}", channelHandler.PutMember)
					r.Delete("/channels/{id}/members/{userID}", channelHandler.DeleteMember)
					if sessionHandler != nil {
						r.Get("/channels/{id}/sessions", sessionHandler.ListChannelSessions)
					}
				})
			}
			if sessionHandler != nil {
				mountSessionRoutes(r, sessionHandler)
			}
			if memoryHandler != nil {
				mountMemoryRoutes(r, database, memoryHandler)
			}

			r.Group(func(r chi.Router) {
				// Escalation-sensitive: JWT callers must be org admins; scoped API-key
				// callers pass (admin gate skipped for keys). Reads stay member-visible.
				r.Use(middleware.RequireAPIKeyScopeOrJWT("credentials"))
				r.Get("/credentials", credHandler.List)
				r.Get("/credentials/{id}", credHandler.Get)
				r.Group(func(r chi.Router) {
					r.Use(middleware.RequireOrgAdminOrAPIKey(database))
					r.Post("/credentials", credHandler.Create)
					r.Delete("/credentials/{id}", credHandler.Revoke)
				})
			})

			r.Group(func(r chi.Router) {
				// Escalation-sensitive (as credentials above): admin-gate JWT, allow keys.
				r.Use(middleware.RequireAPIKeyScopeOrJWT("tokens"))
				r.Group(func(r chi.Router) {
					r.Use(middleware.RequireOrgAdminOrAPIKey(database))
					// List leaks org-wide minted-token inventory: admin-gate it
					// alongside mint/revoke (API keys with scope still pass).
					r.Get("/tokens", tokenHandler.List)
					r.Post("/tokens", tokenHandler.Mint)
					r.Delete("/tokens/{jti}", tokenHandler.Revoke)
				})
			})

			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireAPIKeyScopeOrJWT("agents"))
				if databaseIntegrationHandler != nil {
					r.Group(func(r chi.Router) {
						r.Use(middleware.RequireOrgAdmin(database))
						r.Post("/database-integrations", databaseIntegrationHandler.Create)
						r.Post("/database-integrations/{id}/test", databaseIntegrationHandler.Test)
						r.Post("/database-integrations/{id}/introspect", databaseIntegrationHandler.Introspect)
						r.Patch("/database-integrations/{id}/name", databaseIntegrationHandler.Rename)
						r.Put("/database-integrations/{id}/policy", databaseIntegrationHandler.UpdatePolicy)
						r.Delete("/database-integrations/{id}", databaseIntegrationHandler.Revoke)
					})
				}
				mountSandboxTemplateRoutes(r, database, sandboxTemplateHandler)
				triggerDeliveryHandler := handler.NewTriggerDeliveryHandler(database)
				triggerOptions := []handler.TriggerHandlerOption{handler.WithTriggerWebhookBaseURL(cfg.APIWebhookBaseURL)}
				if slackChannelHandler != nil {
					triggerOptions = append(triggerOptions, handler.WithTriggerExternalProvisioner(slackChannelHandler))
				}
				triggerHandler := handler.NewTriggerHandler(database, triggerOptions...)
				scheduleHandler := handler.NewScheduleHandler(database)
				if agentHandler != nil {
					r.Get("/agents", agentHandler.List)
					r.Get("/agents/catalog", agentHandler.ListCatalog)
					r.Get("/agents/catalog/{slug}", agentHandler.GetCatalog)
					r.Get("/agents/models", agentHandler.ListModels)
					r.Get("/agents/{id}", agentHandler.Get)
					r.Get("/triggers", triggerHandler.List)
					r.Get("/triggers/{id}", triggerHandler.Get)
					r.Get("/schedules", scheduleHandler.List)
					r.Get("/schedules/{id}", scheduleHandler.Get)
					r.Get("/agents/{id}/trigger-deliveries", triggerDeliveryHandler.List)
					r.Get("/agents/{id}/trigger-deliveries/{deliveryID}", triggerDeliveryHandler.Get)
					// Agent create/update/archive are TEAM-MEMBER actions: the handlers
					// enforce that the actor can manage the target team
					// (resolveAndAuthorizeAgentTeam / authorizeAgentMutation), so these
					// routes are intentionally NOT admin-gated.
					r.Post("/agents", agentHandler.Create)
					r.Patch("/agents/{id}", agentHandler.Update)
					r.Delete("/agents/{id}", agentHandler.Archive)
					// Model change is a member action: UpdateModel enforces authorizeAgentMutation, so NOT admin-gated.
					r.Patch("/agents/{id}/model", agentHandler.UpdateModel)
					r.Post("/triggers", triggerHandler.Create)
					r.Post("/schedules", scheduleHandler.Create)
					r.Post("/agents/catalog/{slug}/install", agentHandler.InstallCatalog)
					r.Delete("/agents/catalog/{slug}/install", agentHandler.UninstallCatalog)
					r.Group(func(r chi.Router) {
						r.Use(middleware.RequireOrgAdmin(database))
						r.Put("/agents/{id}/connections/{connectionID}/resources", agentHandler.UpdateConnectionResources)
						r.Patch("/triggers/{id}", triggerHandler.Update)
						r.Delete("/triggers/{id}", triggerHandler.Delete)
						r.Patch("/schedules/{id}", scheduleHandler.Update)
						r.Delete("/schedules/{id}", scheduleHandler.Delete)
					})
				}
				r.Route("/sandboxes", func(r chi.Router) {
					sandboxHandler := handler.NewSandboxHandler(database, orchestrator)
					r.Get("/", sandboxHandler.List)
					r.Get("/{id}", sandboxHandler.Get)
					if orchestrator != nil {
						r.Post("/{id}/stop", sandboxHandler.Stop)
						r.Post("/{id}/exec", sandboxHandler.Exec)
						r.Delete("/{id}", sandboxHandler.Delete)
					}
				})
			})

			mountRAGRoutes(r, database, ragSourceHandler, ragSearchHandler)
			mountUploadRoutes(r, database, uploadsHandler, imageDescribeHandler, transcriptionHandler)
		})
	})
}
