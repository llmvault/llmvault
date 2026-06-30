package main

import (
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
)

// mountUploadRoutes registers org-scoped upload, asset, image, and
// transcription endpoints. Callers must already be inside the authenticated,
// org-resolved route group.
func mountUploadRoutes(
	r chi.Router,
	database *gorm.DB,
	uploadsHandler *handler.UploadsHandler,
	imageDescribeHandler *handler.ImageDescribeHandler,
	transcriptionHandler *handler.TranscriptionHandler,
) {
	if uploadsHandler != nil {
		r.Route("/uploads", func(r chi.Router) {
			r.Use(middleware.ResolveUser(database))
			r.Post("/sign", uploadsHandler.Sign)
			r.Post("/upload", uploadsHandler.Upload)
		})
		r.Get("/assets", uploadsHandler.ListAssets)
		r.Group(func(r chi.Router) {
			r.Use(middleware.ResolveUser(database))
			r.Post("/assets/upload", uploadsHandler.UploadAgentAsset)
		})
	}
	if imageDescribeHandler != nil {
		r.Get("/images/models", imageDescribeHandler.ListGenerationModels)
		r.Post("/images/describe", imageDescribeHandler.Describe)
	}
	if transcriptionHandler != nil {
		r.Post("/transcriptions", transcriptionHandler.Create)
	}
}
