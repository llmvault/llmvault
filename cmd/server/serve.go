package main

import (
	"context"
	"crypto/rsa"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/usehivy/hivy/internal/bootstrap"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/goroutine"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
	sentryobs "github.com/usehivy/hivy/internal/observability/sentry"
)

func runServe(ctx context.Context, deps *bootstrap.Deps, enqueuer enqueue.TaskEnqueuer) error {
	cfg := deps.Config
	database := deps.DB
	redisClient := deps.Redis
	cacheManager := deps.CacheManager
	apiKeyCache := deps.APIKeyCache
	ctr := deps.Counter
	signingKey := deps.SigningKey
	rsaKey := deps.RSAKey
	nangoClient := deps.NangoClient
	actionsCatalog := deps.ActionsCatalog
	sandboxEncKey := deps.SandboxEncKey
	orchestrator := deps.Orchestrator

	logger := slog.Default()

	goroutine.Go(ctx, func(ctx context.Context) {
		if err := cacheManager.Invalidator().Subscribe(ctx); err != nil {
			slog.Error("invalidation subscriber stopped", "error", err)
		}
	})

	h, err := buildServeHandlers(ctx, deps, enqueuer)
	if err != nil {
		return err
	}

	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(middleware.RealIP(cfg.TrustedProxyCIDRs))
	r.Use(sentryobs.Middleware())
	r.Use(sentryobs.Recoverer())
	r.Use(sentryobs.Capture5xxResponses())
	r.Use(middleware.SecurityHeaders())
	r.Use(middleware.CORS(cfg.CORSOrigins, cfg.IsProduction()))
	r.Use(middleware.RequestLog(logger))

	rsaPub := rsaKey.Public().(*rsa.PublicKey)

	// Sandbox orchestration is expected whenever a provider is configured; if it
	// is configured but the orchestrator is nil, the subsystem is silently
	// missing and /readyz must report unavailable.
	orchestratorMissing := cfg.SandboxProviderID != "" && orchestrator == nil

	setupPublicRoutes(r, cfg, database, redisClient, h.providerHandler, h.integrationHandler, actionsCatalog, h.orgInviteHandler, h.nangoWebhookHandler, h.incomingWebhookHandler, nangoClient, sandboxEncKey, deps.KMS, h.uploadsHandler, h.imageDescribeHandler, h.canvasHandler, h.appsInternalHandler, orchestrator, orchestratorMissing, deps.Purchases)
	runtimeIngressHandler := handler.NewRuntimeStreamIngressHandler(database, sandboxEncKey, h.runtimeStreamStore, enqueuer)
	r.Get("/internal/runtime-events/sandboxes/{sandboxID}/sessions/{sessionID}/ws", runtimeIngressHandler.HandleSessionWS)
	r.Get("/internal/runtime-events/sandboxes/{sandboxID}/ws", runtimeIngressHandler.HandleWS)
	r.Post("/internal/runtime-events/sandboxes/{sandboxID}/turn-state", runtimeIngressHandler.HandleTurnState)

	r.Post("/incoming/triggers/{triggerID}", h.httpTriggerHandler.Handle)
	if h.mcpServerHandler != nil {
		r.Get("/v1/mcp-servers/oauth/callback", h.mcpServerHandler.OAuthCallback)
		r.Get("/v1/mcp-servers/oauth/client-metadata", h.mcpServerHandler.OAuthClientMetadata)
	}
	setupAuthRoutes(r, ctx, cfg, rsaPub, h.authHandler, h.oauthHandler)
	registerSheetLiveRoute(r, h.sheetsHandler)
	setupV1Routes(r, cfg, rsaPub, database, apiKeyCache, enqueuer, h.orgHandler, h.orgInviteHandler, h.brandHandler, h.teamHandler, h.usageHandler, h.auditHandler, h.reportingHandler, h.generationHandler, h.apiKeyHandler, h.billingHandler, h.dashboardHandler, h.slackChannelHandler, h.channelHandler, h.sessionHandler, h.memoryHandler, h.credHandler, h.tokenHandler, h.sandboxTemplateHandler, h.databaseIntegrationHandler, h.ragRuntime.sourceHandler, h.ragRuntime.searchHandler, h.uploadsHandler, h.imageDescribeHandler, h.agentHandler, h.canvasHandler, h.sheetsHandler, h.appsHandler, h.transcriptionHandler, h.mcpServerHandler, orchestrator, h.auditWriter)

	setupConnectRoutes(r, cfg, rsaPub, database, h.integrationHandler, h.connectionHandler, h.credHandler)
	setupProxyAndAuxRoutes(r, cfg, deps, signingKey, database, h.proxyHandler, h.auditWriter, h.generationWriter, h.attributionCache, ctr, enqueuer, h.runtimeCompileDeps)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Port),
		Handler: r,
		// ReadHeaderTimeout guards against Slowloris without killing long request
		// bodies (drive uploads); per-handler deadlines use the ctx.
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      0,
		IdleTimeout:       120 * time.Second,
		ErrorLog:          sentryobs.NewStdlogBridge("api_server"),
	}

	goroutine.Go(ctx, func(context.Context) {
		slog.Info("server starting", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
		}
	})

	mcpSrv := setupMCPServer(ctx, cfg, signingKey, database, h.mcpHandler)

	<-ctx.Done()
	shutdownServers(ctx, srv, mcpSrv, h.auditWriter, h.generationWriter, deps)
	return nil
}
