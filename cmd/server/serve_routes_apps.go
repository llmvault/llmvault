package main

import (
	"crypto/rsa"
	"log/slog"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/apps"
	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/sandbox"
	"github.com/usehivy/hivy/internal/sheets"
	"github.com/usehivy/hivy/internal/storage"
)

// buildAppsInternalHandler wires the internal app API handler over the shared
// sheets service. Built even without the sandbox encryption key so the routes
// answer 503 "not configured" instead of 404, mirroring the drive endpoints.
func buildAppsInternalHandler(cfg *config.Config, database *gorm.DB, svc *sheets.Service, encKey *crypto.SymmetricKey) *handler.AppsInternalHandler {
	appsHandler := handler.NewAppsInternalHandler(database, svc, encKey)
	if presigner := buildSheetsPresigner(cfg); presigner != nil {
		appsHandler.WithPresigner(presigner)
	}
	return appsHandler
}

// mountInternalAppRoutes registers the app-secret-authenticated internal app
// API (apps plan §1.2) alongside the other /internal sandbox-facing endpoints.
// Auth is per-request bearer verification inside the handler (authApp), so no
// middleware wraps the group. The surface is full row CRUD on the app's one
// bound sheet plus read-only structure — no schema mutation routes exist.
func mountInternalAppRoutes(r chi.Router, appsHandler *handler.AppsInternalHandler) {
	if appsHandler == nil {
		return
	}
	r.Route("/internal/apps/{appID}/v1", func(r chi.Router) {
		r.Get("/sheet", appsHandler.SheetStructure)
		r.Route("/pages/{pageID}", func(r chi.Router) {
			r.Post("/rows/query", appsHandler.QueryRows)
			r.Post("/rows", appsHandler.InsertRows)
			r.Patch("/rows", appsHandler.UpdateRows)
			r.Delete("/rows", appsHandler.DeleteRows)
			r.Post("/attachments/download-url", appsHandler.AttachmentDownloadURL)
		})
	})
}

// buildAppsService wires the apps orchestration service (plan §3/§4). Nil
// when sandbox orchestration is not configured — the apps surface is then
// not mounted, exactly like other provider-dependent subsystems.
func buildAppsService(cfg *config.Config, database *gorm.DB, encKey *crypto.SymmetricKey, orchestrator *sandbox.Orchestrator, sheetsService *sheets.Service, rsaKey *rsa.PrivateKey) *apps.Service {
	if orchestrator == nil || orchestrator.Provider() == nil || encKey == nil {
		return nil
	}
	authPublicKeyPEM := ""
	if rsaKey != nil {
		pem, err := apps.EncodeAuthPublicKeyPEM(&rsaKey.PublicKey)
		if err != nil {
			slog.Error("encode auth public key for app sandboxes failed; app deploys disabled", "error", err)
			return nil
		}
		authPublicKeyPEM = pem
	}
	var store apps.ObjectStore
	if cfg.S3Bucket != "" {
		presigner, err := storage.NewS3Presigner(storage.PublicAssetsConfig{
			Bucket:          cfg.S3Bucket,
			Region:          cfg.S3Region,
			Endpoint:        cfg.S3Endpoint,
			PresignEndpoint: cfg.S3PresignEndpoint,
			AccessKey:       cfg.S3AccessKey,
			SecretKey:       cfg.S3SecretKey,
		})
		if err != nil {
			slog.Error("s3 apps object store init failed; app publishing disabled", "error", err)
		} else {
			store = presigner
		}
	}
	return apps.NewService(database, cfg, encKey, orchestrator.Provider(), store, sheetsService, authPublicKeyPEM)
}

// mountAppRoutes registers the org-scoped apps REST surface (plan §7).
// Callers must already be inside the authenticated, org-resolved /v1 group;
// ResolveUser gives channel authorization and the launch JWT a user identity.
func mountAppRoutes(r chi.Router, database *gorm.DB, appsRESTHandler *handler.AppsHandler) {
	if appsRESTHandler == nil {
		return
	}
	r.Route("/apps", func(r chi.Router) {
		r.Use(middleware.ResolveUser(database))
		r.Post("/", appsRESTHandler.Create)
		r.Get("/", appsRESTHandler.List)
		r.Route("/{appID}", func(r chi.Router) {
			r.Get("/", appsRESTHandler.Get)
			r.Delete("/", appsRESTHandler.Archive)
			r.Get("/launch", appsRESTHandler.Launch)
		})
	})
}
