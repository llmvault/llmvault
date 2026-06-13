package main

import (
	"log/slog"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/storage"
)

// buildUploadsHandler wires the public-assets uploads handler when a public
// assets bucket is configured. It returns nil (with /v1/uploads/sign disabled)
// when the bucket is unset or the presigner fails to initialise.
func buildUploadsHandler(cfg *config.Config, database *gorm.DB, sandboxEncKey *crypto.SymmetricKey) *handler.UploadsHandler {
	if cfg.PublicAssetsBucket == "" {
		return nil
	}
	presigner, err := storage.NewS3Presigner(storage.PublicAssetsConfig{
		Bucket:       cfg.PublicAssetsBucket,
		Region:       cfg.PublicAssetsRegion,
		Endpoint:     cfg.PublicAssetsEndpoint,
		AccessKey:    cfg.PublicAssetsAccessKey,
		SecretKey:    cfg.PublicAssetsSecretKey,
		SignTTL:      cfg.PublicAssetsSignTTL,
		UsePublicACL: cfg.PublicAssetsUseACL,
	})
	if err != nil {
		slog.Error("public assets presigner init failed; /v1/uploads/sign disabled", "error", err)
		return nil
	}
	uploadsHandler := handler.NewUploadsHandler(database, presigner)
	uploadsHandler.WithAssetPreviewBaseURL(cfg.APIWebhookBaseURL)
	uploadsHandler.WithRuntimeImages(cfg.SandboxesRuntimeBaseImage)
	if sandboxEncKey != nil {
		uploadsHandler.WithStreamer(presigner, sandboxEncKey)
	}
	slog.Info("public assets uploads ready", "bucket", cfg.PublicAssetsBucket)
	return uploadsHandler
}
