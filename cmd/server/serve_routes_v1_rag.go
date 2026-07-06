package main

import (
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
)

// mountRAGRoutes registers the /rag knowledge-base routes: reads for any org
// member, source mutations and sync/prune jobs gated to org admins. No-op when
// RAG is not configured.
func mountRAGRoutes(
	r chi.Router,
	database *gorm.DB,
	ragSourceHandler *handler.RAGSourceHandler,
	ragSearchHandler *handler.RAGSearchHandler,
) {
	if ragSourceHandler == nil {
		return
	}
	r.Route("/rag", func(r chi.Router) {
		r.Use(middleware.ResolveUser(database))
		// Reads stay visible to any org member.
		r.Get("/integrations", ragSourceHandler.ListIntegrations)
		r.Get("/connections/{connection_id}/scopes", ragSourceHandler.ListConnectionScopes)
		r.Get("/sources", ragSourceHandler.List)
		r.Get("/sources/{id}", ragSourceHandler.Get)
		r.Get("/sources/{id}/attempts", ragSourceHandler.ListAttempts)
		r.Get("/sources/{id}/attempts/{attempt_id}", ragSourceHandler.GetAttempt)
		r.Get("/sources/{id}/channels", ragSourceHandler.GetSourceChannels)
		if ragSearchHandler != nil {
			r.Post("/search", ragSearchHandler.Search)
			r.Get("/sources/{id}/documents", ragSearchHandler.ListDocuments)
		}
		// Mutations (sources, sync/prune jobs) are admin-only: a non-admin
		// must not reconfigure org-wide RAG ingestion.
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireOrgAdmin(database))
			r.Post("/sources", ragSourceHandler.Create)
			r.Patch("/sources/{id}", ragSourceHandler.Update)
			r.Delete("/sources/{id}", ragSourceHandler.Delete)
			r.Put("/sources/{id}/channels", ragSourceHandler.SetSourceChannels)
			r.Post("/sources/{id}/sync", ragSourceHandler.TriggerSync)
			r.Post("/sources/{id}/prune", ragSourceHandler.TriggerPrune)
			r.Post("/website/discover-sections", ragSourceHandler.DiscoverWebsiteSections)
		})
	})
}
