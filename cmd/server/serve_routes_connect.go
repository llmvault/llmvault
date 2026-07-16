package main

import (
	"crypto/rsa"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
)

func setupConnectRoutes(
	r chi.Router,
	cfg *config.Config,
	rsaPub *rsa.PublicKey,
	database *gorm.DB,
	integrationHandler *handler.IntegrationHandler,
	connectionHandler *handler.ConnectionHandler,
	credentialHandler *handler.CredentialHandler,
) {
	if cfg.AdminEnabled {
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireAdminSecret(cfg.AdminSecret))
			r.Get("/v1/admin/integrations", integrationHandler.ListAdmin)
			r.Put("/v1/admin/integrations/{id}", integrationHandler.UpsertAdmin)
			r.Delete("/v1/admin/integrations/{id}", integrationHandler.DeleteAdmin)
			r.Get("/v1/admin/system-credentials", credentialHandler.ListSystem)
			r.Post("/v1/admin/system-credentials", credentialHandler.CreateSystem)
			r.Patch("/v1/admin/system-credentials/{id}", credentialHandler.UpdateSystem)
			r.Delete("/v1/admin/system-credentials/{id}", credentialHandler.RevokeSystem)
			r.Get("/v1/admin/llm-providers", credentialHandler.ListLLMProviders)
		})
	}

	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireAuth(rsaPub, cfg.AuthIssuer, cfg.AuthAudience))
		r.Use(middleware.RequireEmailConfirmed(database))
		r.Use(middleware.ResolveUser(database))
		r.Group(func(r chi.Router) {
			r.Use(middleware.ResolveOrgFlexible(database))
			// Connections are org-wide integration credentials: creating,
			// reconnecting, revoking, and enumerating them is admin-only. A
			// non-admin member must not manage or list the org's connections.
			r.Use(middleware.RequireOrgAdmin(database))
			r.Post("/v1/integrations/{id}/connect-session", connectionHandler.CreateConnectSession)
			r.Post("/v1/integrations/{id}/connections", connectionHandler.Create)
			r.Get("/v1/connections", connectionHandler.List)
			r.Get("/v1/connections/{id}", connectionHandler.Get)
			r.Patch("/v1/connections/{id}/name", connectionHandler.Rename)
			r.Put("/v1/connections/{id}/resources", connectionHandler.UpdateResources)
			r.Get("/v1/connections/{id}/resources/{type}", connectionHandler.ListResources)
			r.Post("/v1/connections/{id}/reconnect-session", connectionHandler.CreateReconnectSession)
			r.Patch("/v1/connections/{id}/webhook-configured", connectionHandler.MarkWebhookConfigured)
			r.Delete("/v1/connections/{id}", connectionHandler.Revoke)
		})
	})
}
