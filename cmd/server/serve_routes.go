package main

import (
	"context"
	"crypto/rsa"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/mcp/catalog"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/nango"
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
	plansHandler *handler.PlansHandler,
	agentOutboundWebhookHandler *handler.AgentOutboundWebhookHandler,
	nangoWebhookHandler *handler.NangoWebhookHandler,
	incomingWebhookHandler *handler.IncomingWebhookHandler,
	nangoClient *nango.Client,
	sandboxEncKey *crypto.SymmetricKey,
	kms *crypto.KeyWrapper,
	uploadsHandler *handler.UploadsHandler,
	sqliteBackupHandler *handler.AgentSQLiteBackupHandler,
	orchestratorMissing bool,
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
	r.Get("/v1/catalog/integrations", actionsHandler.ListIntegrations)
	r.Get("/v1/catalog/integrations/{id}", actionsHandler.GetIntegration)
	r.Get("/v1/catalog/integrations/{id}/actions", actionsHandler.ListActions)
	r.Get("/v1/catalog/integrations/{id}/triggers", actionsHandler.ListTriggers)
	r.Get("/v1/catalog/integrations/{id}/schema-paths", actionsHandler.GetSchemaPaths)

	// Org invite preview (public, token-based lookup)
	r.Get("/v1/invites/{token}", orgInviteHandler.Preview)

	// Billing plans catalog (no auth)
	r.Get("/v1/plans", plansHandler.List)
	if uploadsHandler != nil {
		r.Get("/v1/assets/preview", uploadsHandler.PreviewAsset)
	}

	// Webhook receivers (HMAC-verified, no auth middleware)
	r.Post("/internal/webhooks/agent/{sandboxID}", agentOutboundWebhookHandler.Handle)
	r.Post("/internal/webhooks/agent/{sandboxID}/batch", agentOutboundWebhookHandler.HandleBatch)
	r.Post("/internal/webhooks/nango", nangoWebhookHandler.Handle)

	// Sandbox proxy endpoints (bearer-token auth, no middleware)
	if nangoClient != nil && sandboxEncKey != nil {
		gitCredsHandler := handler.NewGitCredentialsHandler(database, sandboxEncKey, nangoClient)
		r.Post("/internal/git-credentials/{agentID}", gitCredsHandler.Handle)

		railwayProxyHandler := handler.NewRailwayProxyHandler(database, sandboxEncKey, nangoClient)
		r.Post("/internal/railway-proxy/{agentID}", railwayProxyHandler.Handle)

		bugsinkProxyHandler := handler.NewBugsinkProxyHandler(database, sandboxEncKey, nangoClient)
		r.Handle("/internal/bugsink-proxy/{agentID}/*", http.HandlerFunc(bugsinkProxyHandler.Handle))

		glitchTipProxyHandler := handler.NewGlitchTipProxyHandler(database, sandboxEncKey, nangoClient)
		r.Handle("/internal/glitchtip-proxy/{agentID}/*", http.HandlerFunc(glitchTipProxyHandler.Handle))

		linearProxyHandler := handler.NewLinearProxyHandler(database, sandboxEncKey, nangoClient)
		r.Post("/internal/linear-proxy/{agentID}", linearProxyHandler.Handle)

		notionProxyHandler := handler.NewNotionProxyHandler(database, sandboxEncKey, nangoClient)
		r.Handle("/internal/notion-proxy/{agentID}/*", http.HandlerFunc(notionProxyHandler.Handle))

		vercelProxyHandler := handler.NewVercelProxyHandler(database, sandboxEncKey, nangoClient)
		r.Handle("/internal/vercel-proxy/{agentID}/*", http.HandlerFunc(vercelProxyHandler.Handle))

		slackProxyHandler := handler.NewSlackProxyHandler(database, sandboxEncKey, nangoClient)
		r.Handle("/internal/slack-proxy/{agentID}/*", http.HandlerFunc(slackProxyHandler.Handle))
	}
	if sandboxEncKey != nil && kms != nil {
		databaseProxyHandler := handler.NewDatabaseProxyHandler(database, sandboxEncKey, kms)
		r.Post("/internal/database-proxy/postgres/{agentID}", databaseProxyHandler.Handle("postgres"))
		r.Post("/internal/database-proxy/mysql/{agentID}", databaseProxyHandler.Handle("mysql"))
		r.Post("/internal/database-proxy/mongodb/{agentID}", databaseProxyHandler.Handle("mongodb"))
	}

	// Direct incoming webhooks for providers requiring manual webhook configuration
	r.Post("/incoming/webhooks/{provider}/{connectionID}", incomingWebhookHandler.Handle)

	if uploadsHandler != nil {
		r.Put("/internal/agents/{agentID}/drive/*", uploadsHandler.StreamAgentAsset)
		r.Post("/internal/agents/{agentID}/drive/move", uploadsHandler.MoveAgentAsset)
		r.Delete("/internal/agents/{agentID}/drive/*", uploadsHandler.DeleteAgentAsset)
	}

	if sqliteBackupHandler != nil {
		r.Put("/internal/agents/{agentID}/sqlite-backup", sqliteBackupHandler.Upload)
		r.Post("/internal/agents/{agentID}/sqlite-backup/presign", sqliteBackupHandler.Presign)
		r.Post("/internal/agents/{agentID}/sqlite-backup/confirm", sqliteBackupHandler.Confirm)
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
			r.Delete("/me", authHandler.DeleteAccount)
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
