package main

import (
	"log/slog"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/storage"
)

// buildWorkerStorageReader returns the S3 reader backing the sheet CSV
// import worker, or nil when the shared bucket is not configured (which
// leaves the task unregistered).
func buildWorkerStorageReader(cfg *config.Config) storage.Reader {
	if cfg.S3Bucket == "" {
		return nil
	}
	reader, err := storage.NewS3Presigner(storage.PublicAssetsConfig{
		Bucket:          cfg.S3Bucket,
		Region:          cfg.S3Region,
		Endpoint:        cfg.S3Endpoint,
		PresignEndpoint: cfg.S3PresignEndpoint,
		AccessKey:       cfg.S3AccessKey,
		SecretKey:       cfg.S3SecretKey,
	})
	if err != nil {
		slog.Error("worker s3 reader init failed; sheet csv imports disabled", "error", err)
		return nil
	}
	return reader
}
