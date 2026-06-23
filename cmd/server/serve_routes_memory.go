package main

import (
	"github.com/go-chi/chi/v5"

	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
)

func mountMemoryRoutes(r chi.Router, memoryHandler *handler.MemoryHandler) {
	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireAPIKeyScopeOrJWT("memories"))
		r.Get("/memories", memoryHandler.List)
		r.Post("/memories", memoryHandler.Create)
		r.Patch("/memories/{id}", memoryHandler.Update)
		r.Delete("/memories/{id}", memoryHandler.Archive)
	})
}
