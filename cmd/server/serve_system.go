package main

import (
	"net/http"
	"time"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/bootstrap"
	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/proxy"
	"github.com/usehivy/hivy/internal/registry"
	"github.com/usehivy/hivy/internal/system"
)

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
