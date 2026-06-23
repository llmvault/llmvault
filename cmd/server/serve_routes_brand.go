package main

import (
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
)

func mountBrandRoutes(r chi.Router, database *gorm.DB, brandHandler *handler.BrandHandler) {
	if brandHandler == nil {
		return
	}
	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireAPIKeyScopeOrJWT("brands"))
		r.Get("/orgs/current/brands", brandHandler.List)
		r.Get("/orgs/current/brands/{id}", brandHandler.Get)
		r.Get("/orgs/current/brands/{id}/assets", brandHandler.ListAssets)
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireOrgAdminOrAPIKey(database))
			r.Post("/orgs/current/brands", brandHandler.Create)
			r.Patch("/orgs/current/brands/{id}", brandHandler.Update)
			r.Delete("/orgs/current/brands/{id}", brandHandler.Archive)
			r.Post("/orgs/current/brands/{id}/default", brandHandler.SetDefault)
			r.Post("/orgs/current/brands/{id}/assets", brandHandler.CreateAsset)
			r.Delete("/orgs/current/brands/{id}/assets/{assetID}", brandHandler.DeleteAsset)
		})
	})
}
