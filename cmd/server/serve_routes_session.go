package main

import (
	"github.com/go-chi/chi/v5"

	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
)

func mountSessionRoutes(r chi.Router, sessionHandler *handler.SessionHandler) {
	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireAPIKeyScopeOrJWT("sessions"))
		r.Get("/sessions", sessionHandler.List)
		r.Post("/sessions", sessionHandler.Create)
		r.Get("/sessions/{id}", sessionHandler.Get)
		r.Get("/sessions/{id}/usage", sessionHandler.GetUsage)
		r.Get("/sessions/{id}/stream", sessionHandler.Stream)
		r.Get("/sessions/{id}/name-updates", sessionHandler.StreamNameUpdates)
		r.Get("/sessions/{id}/notices", sessionHandler.Notices)
		r.Patch("/sessions/{id}", sessionHandler.Update)
		r.Delete("/sessions/{id}", sessionHandler.Archive)
		r.Post("/sessions/{id}/messages", sessionHandler.SendMessage)
		r.Post("/sessions/{id}/input-responses", sessionHandler.RespondToInput)
		r.Post("/sessions/{id}/transcriptions", sessionHandler.TranscribeAudio)
		r.Post("/sessions/{id}/interrupt", sessionHandler.Interrupt)
		r.Get("/sessions/{id}/events", sessionHandler.ListEvents)
		r.Get("/sessions/{id}/subagents/{childSessionID}/events", sessionHandler.ListSubagentEvents)
		r.Post("/sessions/{id}/sandbox-access", sessionHandler.SandboxAccess)
		r.Post("/sessions/{id}/participants", sessionHandler.AddParticipants)
		r.Put("/sessions/{id}/participants/{userID}", sessionHandler.PutParticipant)
		r.Delete("/sessions/{id}/participants/{userID}", sessionHandler.DeleteParticipant)
	})
}
