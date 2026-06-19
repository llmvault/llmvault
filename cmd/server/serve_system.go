package main

import (
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/bootstrap"
	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/credentials"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/mcp/catalog"
	"github.com/usehivy/hivy/internal/proxy"
	"github.com/usehivy/hivy/internal/registry"
	"github.com/usehivy/hivy/internal/system"
	// Blank import: each task file's init() registers itself in
	// system.registry.
	_ "github.com/usehivy/hivy/internal/system/tasks"
)

func buildSystemTaskHandler(db *gorm.DB, deps *bootstrap.Deps, redisClient *redis.Client) *handler.SystemTaskHandler {
	picker := credentials.NewPicker(db)
	httpClient := &http.Client{
		Transport: &proxy.CaptureTransport{Inner: proxy.NewTransport()},
	}
	gateway := system.NewGenkitGateway(httpClient)
	cache := system.NewRedisCache(redisClient)
	return handler.NewSystemTaskHandler(
		db,
		picker,
		deps.KMS,
		registry.Global(),
		cache,
		gateway,
		deps.Credits,
		catalog.Global(),
	)
}

func buildImageDescribeHandler(db *gorm.DB, cfg *config.Config, deps *bootstrap.Deps) *handler.ImageDescribeHandler {
	httpClient := &http.Client{
		Transport: &proxy.CaptureTransport{Inner: proxy.NewTransport()},
		Timeout:   65 * time.Second,
	}
	gateway := system.NewGenkitGateway(httpClient)
	return handler.NewImageDescribeHandler(
		db,
		deps.KMS,
		registry.Global(),
		gateway,
		cfg.APIWebhookBaseURL,
	)
}
