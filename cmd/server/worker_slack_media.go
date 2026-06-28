package main

import (
	"net/http"
	"time"

	"github.com/usehivy/hivy/internal/bootstrap"
	"github.com/usehivy/hivy/internal/proxy"
	"github.com/usehivy/hivy/internal/system"
	"github.com/usehivy/hivy/internal/tasks"
	"github.com/usehivy/hivy/internal/transcription"
)

func buildWorkerSlackMediaEnricher(deps *bootstrap.Deps) tasks.SlackMediaEnricher {
	modelHTTPClient := &http.Client{
		Transport: &proxy.CaptureTransport{Inner: proxy.NewTransport()},
		Timeout:   65 * time.Second,
	}
	mediaHTTPClient := &http.Client{
		Transport: proxy.NewTransport(),
		Timeout:   65 * time.Second,
	}
	return tasks.NewSlackMediaEnricher(tasks.SlackMediaEnricherConfig{
		DB:          deps.DB,
		KMS:         deps.KMS,
		Registry:    deps.Registry,
		Gateway:     system.NewGenkitGateway(modelHTTPClient),
		Transcriber: transcription.NewElevenLabsTranscriber(modelHTTPClient, time.Minute),
		HTTPClient:  mediaHTTPClient,
	})
}
