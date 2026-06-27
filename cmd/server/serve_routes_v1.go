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
	skillHandler *handler.SkillHandler,
	pluginHandler *handler.PluginHandler,
	databaseIntegrationHandler *handler.DatabaseIntegrationHandler,
	ragSourceHandler *handler.RAGSourceHandler,
	ragSearchHandler *handler.RAGSearchHandler,
	uploadsHandler *handler.UploadsHandler,
	imageDescribeHandler *handler.ImageDescribeHandler,
	systemTaskHandler *handler.SystemTaskHandler,
	agentHandler *handler.AgentHandler,
	canvasHandler *handler.CanvasHandler,
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
			r.Get("/orgs/current/members", orgInviteHandler.ListMembers)
			mountBrandRoutes(r, database, brandHandler)

			// Admin-only org invite management.
			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireOrgAdmin(database))
				r.Patch("/orgs/current", orgHandler.Update)
				r.Get("/orgs/current/environment-variables", orgHandler.ListEnvironmentVariables)
				r.Post("/orgs/current/environment-variables", orgHandler.CreateEnvironmentVariable)
				r.Patch("/orgs/current/environment-variables/{name}", orgHandler.UpdateEnvironmentVariable)
				r.Delete("/orgs/current/environment-variables/{name}", orgHandler.DeleteEnvironmentVariable)
				r.Post("/orgs/current/invites", orgInviteHandler.Create)
				r.Get("/orgs/current/invites", orgInviteHandler.List)
				r.Delete("/orgs/current/invites/{id}", orgInviteHandler.Revoke)
				r.Post("/orgs/current/invites/{id}/resend", orgInviteHandler.Resend)
				if teamHandler != nil {
					r.Get("/orgs/current/teams", teamHandler.List)
					r.Post("/orgs/current/teams", teamHandler.Create)
					r.Get("/orgs/current/teams/{id}", teamHandler.Get)
					r.Patch("/orgs/current/teams/{id}", teamHandler.Update)
					r.Delete("/orgs/current/teams/{id}", teamHandler.Archive)
					r.Put("/orgs/current/teams/{id}/members/{userID}", teamHandler.PutMember)
					r.Delete("/orgs/current/teams/{id}/members/{userID}", teamHandler.DeleteMember)
				}
			})
			r.Get("/usage", usageHandler.Get)
			if dashboardHandler != nil {
				r.Get("/dashboard", dashboardHandler.Get)
			}
			r.Get("/audit", auditHandler.List)
			r.Get("/reporting", reportingHandler.Get)
			r.Get("/generations", generationHandler.List)
			r.Get("/generations/{id}", generationHandler.Get)
			if canvasHandler != nil {
				r.Get("/canvas/projects", canvasHandler.ListProjects)
				r.Get("/canvas/artifacts", canvasHandler.ListArtifacts)
				r.Get("/canvas/artifacts/{artifactID}", canvasHandler.GetArtifact)
				r.Post("/canvas/artifacts/{artifactID}/preview-url", canvasHandler.PreviewArtifactURL)
			}
			if databaseIntegrationHandler != nil {
				r.Get("/database-integrations", databaseIntegrationHandler.List)
			}

			r.Get("/api-keys", apiKeyHandler.List)
			// Escalation-sensitive: JWT callers must be org admins; API-key callers
			// may only mint keys within their own scopes (APIKeyHandler.Create).
			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireOrgAdminOrAPIKey(database))
				r.Post("/api-keys", apiKeyHandler.Create)
				r.Delete("/api-keys/{id}", apiKeyHandler.Revoke)
			})

			mountBillingRoutes(r, billingHandler, subscriptionHandler)
			if pluginHandler != nil {
				r.Get("/plugins", pluginHandler.List)
				r.Get("/plugins/{slug}", pluginHandler.Get)
				r.Post("/plugins/{slug}/install", pluginHandler.Install)
				r.Delete("/plugins/{slug}/install", pluginHandler.Uninstall)
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
				mountMemoryRoutes(r, memoryHandler)
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
				r.Get("/tokens", tokenHandler.List)
				r.Group(func(r chi.Router) {
					r.Use(middleware.RequireOrgAdminOrAPIKey(database))
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
						r.Put("/database-integrations/{id}/policy", databaseIntegrationHandler.UpdatePolicy)
						r.Delete("/database-integrations/{id}", databaseIntegrationHandler.Revoke)
					})
				}
				r.Route("/sandbox-templates", func(r chi.Router) {
					r.Post("/", sandboxTemplateHandler.Create)
					r.Get("/", sandboxTemplateHandler.List)
					r.Get("/public", sandboxTemplateHandler.ListPublic)
					r.Get("/{id}/build-events", sandboxTemplateHandler.BuildEvents)
					r.Get("/{id}", sandboxTemplateHandler.Get)
					r.Put("/{id}", sandboxTemplateHandler.Update)
					r.Delete("/{id}", sandboxTemplateHandler.Delete)
					r.Post("/{id}/build", sandboxTemplateHandler.TriggerBuild)
					r.Post("/{id}/retry", sandboxTemplateHandler.RetryBuild)
				})
				triggerDeliveryHandler := handler.NewTriggerDeliveryHandler(database)
				if agentHandler != nil {
					r.Get("/agents", agentHandler.List)
					r.Get("/agents/catalog", agentHandler.ListCatalog)
					r.Get("/agents/catalog/{slug}", agentHandler.GetCatalog)
					r.Get("/agents/models", agentHandler.ListModels)
					r.Get("/agents/{id}", agentHandler.Get)
					if pluginHandler != nil {
						r.Get("/agents/{id}/plugins", pluginHandler.ListAgentPlugins)
						r.Post("/agents/{id}/plugins/{slug}", pluginHandler.EnableForAgent)
						r.Delete("/agents/{id}/plugins/{slug}", pluginHandler.DisableForAgent)
					}
					r.Get("/agents/{id}/trigger-deliveries", triggerDeliveryHandler.List)
					r.Get("/agents/{id}/trigger-deliveries/{deliveryID}", triggerDeliveryHandler.Get)
					r.Group(func(r chi.Router) {
						r.Use(middleware.RequireOrgAdmin(database))
						r.Post("/agents", agentHandler.Create)
						r.Post("/agents/catalog/{slug}/install", agentHandler.InstallCatalog)
						r.Delete("/agents/catalog/{slug}/install", agentHandler.UninstallCatalog)
						r.Patch("/agents/{id}", agentHandler.Update)
						r.Delete("/agents/{id}", agentHandler.Archive)
						r.Patch("/agents/{id}/model", agentHandler.UpdateModel)
						r.Put("/agents/{id}/connections/{connectionID}/resources", agentHandler.UpdateConnectionResources)
					})
				}
				if systemTaskHandler != nil {
					r.Post("/system/tasks/{taskName}", systemTaskHandler.Run)
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

			if ragSourceHandler != nil {
				r.Route("/rag", func(r chi.Router) {
					r.Use(middleware.ResolveUser(database))
					// Reads stay visible to any org member.
					r.Get("/integrations", ragSourceHandler.ListIntegrations)
					r.Get("/sources", ragSourceHandler.List)
					r.Get("/sources/{id}", ragSourceHandler.Get)
					r.Get("/sources/{id}/attempts", ragSourceHandler.ListAttempts)
					r.Get("/sources/{id}/attempts/{attempt_id}", ragSourceHandler.GetAttempt)
					if ragSearchHandler != nil {
						r.Post("/search", ragSearchHandler.Search)
					}
					// Mutations (sources, sync/prune jobs) are admin-only: a non-admin
					// must not reconfigure org-wide RAG ingestion.
					r.Group(func(r chi.Router) {
						r.Use(middleware.RequireOrgAdmin(database))
						r.Post("/sources", ragSourceHandler.Create)
						r.Patch("/sources/{id}", ragSourceHandler.Update)
						r.Delete("/sources/{id}", ragSourceHandler.Delete)
						r.Post("/sources/{id}/sync", ragSourceHandler.TriggerSync)
						r.Post("/sources/{id}/prune", ragSourceHandler.TriggerPrune)
						r.Post("/sources/{id}/perm-sync", ragSourceHandler.TriggerPermSync)
					})
				})
			}

			if uploadsHandler != nil {
				r.Route("/uploads", func(r chi.Router) {
					r.Use(middleware.ResolveUser(database))
					r.Post("/sign", uploadsHandler.Sign)
					r.Post("/upload", uploadsHandler.Upload)
				})
				r.Get("/assets", uploadsHandler.ListAssets)
				r.Group(func(r chi.Router) {
					r.Use(middleware.ResolveUser(database))
					r.Post("/assets/upload", uploadsHandler.UploadAgentAsset)
				})
			}
			if imageDescribeHandler != nil {
				r.Get("/images/models", imageDescribeHandler.ListGenerationModels)
				r.Post("/images/describe", imageDescribeHandler.Describe)
			}
		})
	})
}
