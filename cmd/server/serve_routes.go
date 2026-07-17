package main

import (
	"context"
	"crypto/rsa"

	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/billing/purchase"
	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/mcp/catalog"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/nango"
	"github.com/usehivy/hivy/internal/sandbox"
)

func setupPublicRoutes(
	r chi.Router,
	cfg *config.Config,
	database *gorm.DB,
	redisClient *redis.Client,
	providerHandler *handler.ProviderHandler,
	integrationHandler *handler.IntegrationHandler,
	actionsCatalog *catalog.Catalog,
	orgInviteHandler *handler.OrgInviteHandler,
	nangoWebhookHandler *handler.NangoWebhookHandler,
	incomingWebhookHandler *handler.IncomingWebhookHandler,
	nangoClient *nango.Client,
	sandboxEncKey *crypto.SymmetricKey,
	kms *crypto.KeyWrapper,
	uploadsHandler *handler.UploadsHandler,
	imageDescribeHandler *handler.ImageDescribeHandler,
	canvasHandler *handler.CanvasHandler,
	appsInternalHandler *handler.AppsInternalHandler,
	orchestrator *sandbox.Orchestrator,
	orchestratorMissing bool,
	purchases *purchase.Service,
) {
	r.Get("/healthz", healthz)
	r.Get("/readyz", readyz(database, redisClient, orchestratorMissing))

	// Provider discovery (no auth)
	r.Get("/v1/providers", providerHandler.List)
	r.Get("/v1/providers/{id}", providerHandler.Get)
	r.Get("/v1/providers/{id}/models", providerHandler.Models)
	r.Get("/v1/models", providerHandler.AllModels)

	// Integration discovery (no auth)
	r.Get("/v1/integrations/available", integrationHandler.ListAvailable)

	// Integration catalog discovery (no auth)
	actionsHandler := handler.NewActionsHandler(actionsCatalog)
	automationCatalogHandler := handler.NewAutomationCatalogHandler("global/triggers", "global/schedules")
	r.Get("/v1/catalog/integrations", actionsHandler.ListIntegrations)
	r.Get("/v1/catalog/integrations/{id}", actionsHandler.GetIntegration)
	r.Get("/v1/catalog/integrations/{id}/actions", actionsHandler.ListActions)
	r.Get("/v1/catalog/integrations/{id}/triggers", actionsHandler.ListTriggers)
	r.Get("/v1/catalog/integrations/{id}/schema-paths", actionsHandler.GetSchemaPaths)
	r.Get("/v1/catalog/triggers", automationCatalogHandler.ListTriggers)
	r.Get("/v1/catalog/automations", automationCatalogHandler.ListAutomations)

	// Org invite preview (public, token-based lookup)
	r.Get("/v1/invites/{token}", orgInviteHandler.Preview)

	if uploadsHandler != nil {
		r.Get("/v1/assets/preview", uploadsHandler.PreviewAsset)
	}

	// Webhook receivers (HMAC-verified, no auth middleware)
	r.Post("/internal/webhooks/nango", nangoWebhookHandler.Handle)
	if cfg.PaystackSecretKey != "" && purchases != nil {
		paystackWebhookHandler := handler.NewPaystackWebhookHandler(cfg.PaystackSecretKey, purchases)
		r.Post("/internal/webhooks/paystack", paystackWebhookHandler.Handle)
	}
	if cfg.PreviewActivityToken != "" {
		previewActivityHandler := handler.NewPreviewActivityHandler(database, orchestrator, cfg.PreviewActivityToken)
		r.Post("/internal/preview/sandboxes/{externalID}/activity", previewActivityHandler.Handle)
	}

	// Sandbox credential endpoints (bearer-token auth, no middleware)
	if nangoClient != nil && sandboxEncKey != nil {
		gitCredsHandler := handler.NewGitCredentialsHandler(database, sandboxEncKey, nangoClient)
		r.Post("/internal/git-credentials/{agentID}", gitCredsHandler.Handle)

		githubPRCreatedHandler := handler.NewGitHubPRCreatedHandler(database, sandboxEncKey)
		r.Post("/internal/github-pr-created/{agentID}", githubPRCreatedHandler.Handle)
	}
	if sandboxEncKey != nil && kms != nil {
		databaseProxyHandler := handler.NewDatabaseProxyHandler(database, sandboxEncKey, kms)
		r.Post("/internal/database-proxy/postgres/{agentID}", databaseProxyHandler.Handle("postgres"))
		r.Post("/internal/database-proxy/mysql/{agentID}", databaseProxyHandler.Handle("mysql"))
		r.Post("/internal/database-proxy/mongodb/{agentID}", databaseProxyHandler.Handle("mongodb"))
		r.Post("/internal/database-proxy/redis/{agentID}", databaseProxyHandler.Handle("redis"))
	}

	// Direct incoming webhooks for providers requiring manual webhook configuration
	r.Post("/incoming/webhooks/{provider}/{connectionID}", incomingWebhookHandler.Handle)

	if uploadsHandler != nil {
		r.Put("/internal/agents/{agentID}/sandboxes/{sandboxID}/drive/*", uploadsHandler.StreamAgentAsset)
		r.Post("/internal/agents/{agentID}/sandboxes/{sandboxID}/drive/move", uploadsHandler.MoveAgentAsset)
		r.Delete("/internal/agents/{agentID}/sandboxes/{sandboxID}/drive/*", uploadsHandler.DeleteAgentAsset)
		// App template distribution for builder agents: same runtime-secret
		// auth as the drive; builders swap the /drive suffix of
		// HIVY_DRIVE_UPLOAD_URL for /apps-template.zip.
		r.Get("/internal/agents/{agentID}/sandboxes/{sandboxID}/apps-template.zip", uploadsHandler.StreamAppsTemplateZip)
		// Preview side channel: `make preview` in a builder sandbox fetches
		// the app's runtime env + the server-computed public preview URL for
		// a port on the builder's own sandbox. Secrets travel only in this
		// response — never through the model context.
		r.Get("/internal/agents/{agentID}/sandboxes/{sandboxID}/apps/{appID}/preview-env", uploadsHandler.AppPreviewEnv)
	}

	// Internal app API: app-secret bearer auth, sheets CRUD on the app's one
	// bound sheet (apps plan §1.2).
	mountInternalAppRoutes(r, appsInternalHandler)
	if imageDescribeHandler != nil {
		r.Post("/internal/agents/{agentID}/sandboxes/{sandboxID}/images/describe", imageDescribeHandler.DescribeForRuntime)
	}

	if canvasHandler != nil {
		r.Get("/internal/agents/{agentID}/canvas/projects", canvasHandler.ListAgentProjects)
		r.Post("/internal/agents/{agentID}/canvas/projects", canvasHandler.CreateAgentProject)
		r.Get("/internal/agents/{agentID}/canvas/artifacts", canvasHandler.ListAgentArtifacts)
		r.Get("/internal/agents/{agentID}/canvas/snapshot", canvasHandler.SnapshotAgentCanvas)
		r.Post("/internal/agents/{agentID}/canvas/artifacts/sync", canvasHandler.SyncAgentArtifact)
		r.Get("/internal/agents/{agentID}/canvas/brands", canvasHandler.ListAgentBrands)
		r.Post("/internal/agents/{agentID}/canvas/brands", canvasHandler.CreateAgentBrand)
		r.Get("/internal/agents/{agentID}/canvas/brands/{id}", canvasHandler.GetAgentBrand)
		r.Patch("/internal/agents/{agentID}/canvas/brands/{id}", canvasHandler.UpdateAgentBrand)
	}

}

func setupAuthRoutes(
	r chi.Router,
	ctx context.Context,
	cfg *config.Config,
	rsaPub *rsa.PublicKey,
	authHandler *handler.AuthHandler,
	oauthHandler *handler.OAuthHandler,
) {
	r.Route("/auth", func(r chi.Router) {
		r.Use(middleware.AuthRateLimit(ctx, 10, 20))
		r.Post("/register", authHandler.Register)
		r.Post("/login", authHandler.Login)
		r.Post("/refresh", authHandler.Refresh)
		r.Post("/otp/request", authHandler.OTPRequest)
		r.Post("/otp/verify", authHandler.OTPVerify)
		r.Post("/confirm-email", authHandler.ConfirmEmail)
		r.Post("/resend-confirmation", authHandler.ResendConfirmation)
		r.Post("/forgot-password", authHandler.ForgotPassword)
		r.Post("/reset-password", authHandler.ResetPassword)
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireAuth(rsaPub, cfg.AuthIssuer, cfg.AuthAudience))
			r.Post("/logout", authHandler.Logout)
			r.Get("/me", authHandler.Me)
			r.Patch("/me", authHandler.UpdateProfile)
			// Account deletion removed by design: the product never hard-deletes a
			// users row. Offboarding is member deactivation (org members Remove).
			r.Post("/change-password", authHandler.ChangePassword)
		})
	})

	r.Route("/oauth", func(r chi.Router) {
		r.Use(middleware.AuthRateLimit(ctx, 10, 20))
		r.Get("/github", oauthHandler.GitHubLogin)
		r.Get("/github/callback", oauthHandler.GitHubCallback)
		r.Get("/google", oauthHandler.GoogleLogin)
		r.Get("/google/callback", oauthHandler.GoogleCallback)
		r.Get("/x", oauthHandler.XLogin)
		r.Get("/x/callback", oauthHandler.XCallback)
		r.Post("/exchange", oauthHandler.Exchange)
	})
}
