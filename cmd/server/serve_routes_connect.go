package main

import (
	"crypto/rsa"
	"strings"

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
	platformAdminEmails []string,
	integrationHandler *handler.IntegrationHandler,
	connectionHandler *handler.ConnectionHandler,
) {
	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireAuth(rsaPub, cfg.AuthIssuer, cfg.AuthAudience))
		r.Use(middleware.RequireEmailConfirmed(database))
		r.Use(middleware.ResolveUser(database))
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequirePlatformAdmin(platformAdminEmails))
			if strings.TrimSpace(cfg.AdminSecret) != "" {
				r.Group(func(r chi.Router) {
					r.Use(middleware.RequireAdminSecret(cfg.AdminSecret))
					r.Get("/v1/admin/integrations", integrationHandler.ListAdmin)
					r.Put("/v1/admin/integrations/{id}", integrationHandler.UpsertAdmin)
				})
			}
			r.Post("/v1/integrations", integrationHandler.Create)
			r.Get("/v1/integrations", integrationHandler.List)
			r.Get("/v1/integrations/{id}", integrationHandler.Get)
			r.Put("/v1/integrations/{id}", integrationHandler.Update)
			r.Delete("/v1/integrations/{id}", integrationHandler.Delete)
		})
		r.Group(func(r chi.Router) {
			r.Use(middleware.ResolveOrgFlexible(database))
			r.Post("/v1/integrations/{id}/connect-session", connectionHandler.CreateConnectSession)
			r.Post("/v1/integrations/{id}/connections", connectionHandler.Create)
			r.Get("/v1/connections", connectionHandler.List)
			r.Get("/v1/connections/{id}", connectionHandler.Get)
			r.Get("/v1/connections/{id}/resources/{type}", connectionHandler.ListResources)
			r.Post("/v1/connections/{id}/reconnect-session", connectionHandler.CreateReconnectSession)
			r.Patch("/v1/connections/{id}/webhook-configured", connectionHandler.MarkWebhookConfigured)
			r.Delete("/v1/connections/{id}", connectionHandler.Revoke)
		})
	})
}
