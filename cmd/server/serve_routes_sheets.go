package main

import (
	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/sheets"
	"github.com/usehivy/hivy/internal/tasks"
)

// buildSheetsService constructs the single sheets service shared by the REST
// handlers and the MCP tool group, so both surfaces emit realtime events and
// enqueue the CSV import worker. Without the enqueuer, import jobs persist as
// pending and never run.
func buildSheetsService(database *gorm.DB, redisClient redis.UniversalClient, enqueuer enqueue.TaskEnqueuer) *sheets.Service {
	svc := sheets.NewService(database)
	if redisClient != nil {
		svc.WithPublisher(sheets.NewRedisEventPublisher(redisClient))
	}
	if enqueuer != nil {
		svc.WithImportEnqueuer(tasks.NewSheetImportEnqueuer(enqueuer))
	}
	return svc
}

// buildSheetsHandler wires the shared sheets service with the REST handler,
// its Redis live-stream support, and the attachment presigner.
func buildSheetsHandler(cfg *config.Config, database *gorm.DB, redisClient redis.UniversalClient, signingKey []byte, svc *sheets.Service) *handler.SheetsHandler {
	sheetsHandler := handler.NewSheetsHandler(database, svc, signingKey).
		WithRedis(redisClient)
	if presigner := buildSheetsPresigner(cfg); presigner != nil {
		sheetsHandler.WithPresigner(presigner)
	}
	return sheetsHandler
}

// mountSheetRoutes registers the org-scoped sheets REST surface (plan §2b).
// Callers must already be inside the authenticated, org-resolved /v1 group.
func mountSheetRoutes(r chi.Router, database *gorm.DB, sheetsHandler *handler.SheetsHandler) {
	if sheetsHandler == nil {
		return
	}
	r.Route("/sheets", func(r chi.Router) {
		r.Use(middleware.ResolveUser(database))
		r.Get("/", sheetsHandler.ListSheets)
		r.Post("/", sheetsHandler.CreateSheet)
		r.Get("/imports/{jobID}", sheetsHandler.GetImportJob)
		r.Route("/{sheetID}", func(r chi.Router) {
			r.Use(sheetsHandler.RequireTeamAccess)
			r.Get("/", sheetsHandler.GetSheet)
			r.Patch("/", sheetsHandler.UpdateSheet)
			r.Delete("/", sheetsHandler.ArchiveSheet)
			r.Post("/live-token", sheetsHandler.LiveToken)
			r.Post("/pages", sheetsHandler.CreatePage)
			r.Route("/pages/{pageID}", func(r chi.Router) {
				r.Patch("/", sheetsHandler.UpdatePage)
				r.Delete("/", sheetsHandler.ArchivePage)
				r.Post("/fields", sheetsHandler.CreateField)
				r.Patch("/fields/{fieldID}", sheetsHandler.UpdateField)
				r.Delete("/fields/{fieldID}", sheetsHandler.ArchiveField)
				r.Post("/rows/query", sheetsHandler.QueryRows)
				r.Post("/rows", sheetsHandler.InsertRows)
				r.Patch("/rows", sheetsHandler.UpdateRows)
				r.Delete("/rows", sheetsHandler.DeleteRows)
				r.Get("/views", sheetsHandler.ListViews)
				r.Post("/views", sheetsHandler.CreateView)
				r.Patch("/views/{viewID}", sheetsHandler.UpdateView)
				r.Delete("/views/{viewID}", sheetsHandler.ArchiveView)
				r.Post("/attachments/download-url", sheetsHandler.AttachmentDownloadURL)
				r.Post("/imports", sheetsHandler.CreateImport)
				r.Get("/export.csv", sheetsHandler.ExportCSV)
				r.Get("/operations", sheetsHandler.ListOperations)
				r.Post("/operations/{operationID}/revert", sheetsHandler.RevertOperation)
			})
		})
	})
}

// registerSheetLiveRoute exposes the SSE stream outside the /v1 auth
// middleware: browsers connect directly with the short-lived live token,
// which the handler validates itself. Global CORS covers it.
func registerSheetLiveRoute(r chi.Router, sheetsHandler *handler.SheetsHandler) {
	if sheetsHandler == nil {
		return
	}
	r.Get("/v1/sheets/{sheetID}/live", sheetsHandler.Live)
}
