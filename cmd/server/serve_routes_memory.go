package main

import (
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
)

func mountMemoryRoutes(r chi.Router, database *gorm.DB, memoryHandler *handler.MemoryHandler) {
	directiveHandler := handler.NewDirectiveHandler(database)
	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireAPIKeyScopeOrJWT("memories"))
		r.Get("/memories", memoryHandler.List)
		r.Post("/memories", memoryHandler.Create)
		r.Patch("/memories/{id}", memoryHandler.Update)
		r.Delete("/memories/{id}", memoryHandler.Archive)

		// Directives: hard rules injected verbatim into every prompt in scope.
		r.Get("/directives", directiveHandler.List)
		r.Post("/directives", directiveHandler.Create)
		r.Patch("/directives/{id}", directiveHandler.Update)
		r.Delete("/directives/{id}", directiveHandler.Delete)

		// Observations: the consolidated memory product + human feedback loop.
	})
}
