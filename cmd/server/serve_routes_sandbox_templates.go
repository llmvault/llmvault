package main

import (
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
)

// mountSandboxTemplateRoutes registers /sandbox-templates under the caller's
// group. Reads are open to the enclosing scope; creating/updating/deleting a
// template and kicking off image builds is org-wide runtime config, so those
// mutations are admin-only.
func mountSandboxTemplateRoutes(r chi.Router, db *gorm.DB, h *handler.SandboxTemplateHandler) {
	r.Route("/sandbox-templates", func(r chi.Router) {
		r.Get("/", h.List)
		r.Get("/public", h.ListPublic)
		r.Get("/{id}/build-events", h.BuildEvents)
		r.Get("/{id}", h.Get)
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireOrgAdmin(db))
			r.Post("/", h.Create)
			r.Put("/{id}", h.Update)
			r.Delete("/{id}", h.Delete)
			r.Post("/{id}/build", h.TriggerBuild)
			r.Post("/{id}/retry", h.RetryBuild)
		})
	})
}
