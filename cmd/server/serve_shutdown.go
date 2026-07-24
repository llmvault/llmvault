package main

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/usehivy/hivy/internal/bootstrap"
	"github.com/usehivy/hivy/internal/middleware"
)

// shutdownServers drains the HTTP and MCP servers plus the background writers
// with a bounded deadline once the root context is cancelled.
func shutdownServers(
	ctx context.Context,
	srv *http.Server,
	mcpSrv *http.Server,
	metricsSrv *http.Server,
	auditWriter *middleware.AuditWriter,
	generationWriter *middleware.GenerationWriter,
	deps *bootstrap.Deps,
) {
	slog.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("server shutdown error", "error", err)
	}
	if err := mcpSrv.Shutdown(shutdownCtx); err != nil {
		slog.Error("mcp server shutdown error", "error", err)
	}
	if err := metricsSrv.Shutdown(shutdownCtx); err != nil {
		slog.Error("metrics server shutdown error", "error", err)
	}

	auditWriter.Shutdown(shutdownCtx)
	generationWriter.Shutdown(shutdownCtx)
	if deps.ToolUsageWriter != nil {
		deps.ToolUsageWriter.Shutdown(shutdownCtx)
	}

	slog.Info("serve shutdown complete")
}
