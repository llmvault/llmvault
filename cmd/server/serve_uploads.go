package main

import (
	"log/slog"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/canvasartifact"
	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/storage"
)

// buildUploadsHandler wires upload endpoints when the shared S3 bucket is configured.
func buildUploadsHandler(cfg *config.Config, database *gorm.DB, sandboxEncKey *crypto.SymmetricKey) *handler.UploadsHandler {
	if cfg.S3Bucket == "" {
		return nil
	}
	presigner, err := storage.NewS3Presigner(storage.PublicAssetsConfig{
		Bucket:          cfg.S3Bucket,
		Region:          cfg.S3Region,
		Endpoint:        cfg.S3Endpoint,
		PresignEndpoint: cfg.S3PresignEndpoint,
		AccessKey:       cfg.S3AccessKey,
		SecretKey:       cfg.S3SecretKey,
	})
	if err != nil {
		slog.Error("s3 uploads presigner init failed; uploads disabled", "error", err)
		return nil
	}
	uploadsHandler := handler.NewUploadsHandler(database, presigner)
	uploadsHandler.WithAssetPreviewBaseURL(cfg.APIWebhookBaseURL)
	if sandboxEncKey != nil {
		uploadsHandler.WithStreamer(presigner, sandboxEncKey)
	}
	slog.Info("s3-backed uploads ready", "bucket", cfg.S3Bucket)
	return uploadsHandler
}

// buildSheetsPresigner returns the presigner backing sheet attachment
// download URLs, or nil when the shared bucket is not configured.
func buildSheetsPresigner(cfg *config.Config) storage.Presigner {
	if cfg.S3Bucket == "" {
		return nil
	}
	presigner, err := storage.NewS3Presigner(storage.PublicAssetsConfig{
		Bucket:          cfg.S3Bucket,
		Region:          cfg.S3Region,
		Endpoint:        cfg.S3Endpoint,
		PresignEndpoint: cfg.S3PresignEndpoint,
		AccessKey:       cfg.S3AccessKey,
		SecretKey:       cfg.S3SecretKey,
	})
	if err != nil {
		slog.Error("s3 sheets presigner init failed; attachment downloads disabled", "error", err)
		return nil
	}
	return presigner
}

func buildCanvasArtifactStore(cfg *config.Config) canvasartifact.FileStore {
	if cfg.S3Bucket == "" {
		return nil
	}
	presigner, err := storage.NewS3Presigner(storage.PublicAssetsConfig{
		Bucket:          cfg.S3Bucket,
		Region:          cfg.S3Region,
		Endpoint:        cfg.S3Endpoint,
		PresignEndpoint: cfg.S3PresignEndpoint,
		AccessKey:       cfg.S3AccessKey,
		SecretKey:       cfg.S3SecretKey,
	})
	if err != nil {
		slog.Error("s3 canvas artifact store init failed; canvas artifacts disabled", "error", err)
		return nil
	}
	return presigner
}
