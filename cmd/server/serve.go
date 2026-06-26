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

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/bootstrap"
	"github.com/usehivy/hivy/internal/canvas"
	"github.com/usehivy/hivy/internal/credentials"
	"github.com/usehivy/hivy/internal/email"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/goroutine"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/memory"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	sentryobs "github.com/usehivy/hivy/internal/observability/sentry"
	"github.com/usehivy/hivy/internal/precontext"
	"github.com/usehivy/hivy/internal/proxy"
	"github.com/usehivy/hivy/internal/runtimestream"
	"github.com/usehivy/hivy/internal/sandbox"
	"github.com/usehivy/hivy/internal/spider"
	"github.com/usehivy/hivy/internal/tasks"
	"github.com/usehivy/hivy/internal/transcription"
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
	reg := deps.Registry
	nangoClient := deps.NangoClient
	actionsCatalog := deps.ActionsCatalog
	sandboxEncKey := deps.SandboxEncKey
	orchestrator := deps.Orchestrator
	if orchestrator != nil {
		orchestrator.SetWarmPoolReconciler(func(ctx context.Context, providerID string, profile sandbox.WarmPoolProfile) error {
			return tasks.EnqueueSandboxWarmPoolReconcile(ctx, enqueuer, providerID, profile)
		})
		tasks.EnqueueConfiguredWarmPoolReconciles(ctx, enqueuer, orchestrator)
	}

	logger := slog.Default()

	goroutine.Go(ctx, func(ctx context.Context) {
		if err := cacheManager.Invalidator().Subscribe(ctx); err != nil {
			slog.Error("invalidation subscriber stopped", "error", err)
		}
	})

	auditWriter := middleware.NewAuditWriter(ctx, database, 10000)
	canvasService := canvas.NewService(database, canvas.NewClient(cfg))
	canvasHandler := handler.NewCanvasHandler(database, sandboxEncKey, canvasService)

	generationWriter := middleware.NewGenerationWriter(ctx, database, reg, 10000)
	if enqueuer != nil {
		// Durable fallback: spill billable generation rows to asynq rather than
		// dropping them on a DB blip, full buffer, or shutdown deadline.
		generationWriter.SetEnqueuer(enqueuer)
	}

	mcpHandler := handler.NewMCPHandler(database, signingKey, actionsCatalog, nangoClient, ctr)
	if deps.SpiderClient != nil {
		mcpHandler.SetWebTools(spider.NewWebToolsFunc(deps.SpiderClient))
	}
	runtimeCompileDeps := agentruntime.CompileDeps{
		DB:         database,
		Picker:     credentials.NewPickerWithRegistry(database, reg),
		KMS:        deps.KMS,
		EncKey:     sandboxEncKey,
		SigningKey: signingKey,
		Cfg:        cfg,
		Nango:      nangoClient,
		Canvas:     canvasService,
	}
	if orchestrator != nil {
		orchestrator.SetAgentRuntimeConfigPusher(func(ctx context.Context, sb *model.Sandbox, proxyToken *agentruntime.ProxyTokenResult) error {
			return agentruntime.PushAgentRuntimeConfigForSandboxWithProxyToken(ctx, runtimeCompileDeps, sb, proxyToken)
		})
	}
	credHandler := handler.NewCredentialHandler(database, deps.KMS, cacheManager, ctr)
	databaseIntegrationHandler := handler.NewDatabaseIntegrationHandler(database, deps.KMS)
	tokenHandler := handler.NewTokenHandler(database, signingKey, cacheManager, ctr, actionsCatalog, cfg.MCPBaseURL, mcpHandler.ServerCache)
	providerHandler := handler.NewProviderHandler(reg, database)
	integrationHandler := handler.NewIntegrationHandler(database, nangoClient, actionsCatalog)
	connectionHandler := handler.NewConnectionHandler(database, nangoClient, actionsCatalog, enqueuer)
	orgHandler := handler.NewOrgHandler(database, enqueuer)
	orgHandler.SetEnvironmentEncryptionKey(sandboxEncKey)
	brandHandler := handler.NewBrandHandler(database)
	plansHandler := handler.NewPlansHandler(database)
	var emailSender email.Sender = &email.LogSender{}
	if enqueuer != nil && cfg.ResendAPIKey != "" {
		emailSender = email.NewAsynqSender(enqueuer)
	}
	orgInviteHandler := handler.NewOrgInviteHandler(database, emailSender, cfg.FrontendURL)
	orgInviteHandler.SetEnqueuer(enqueuer)
	authHandler := handler.NewAuthHandler(database, rsaKey, signingKey,
		cfg.AuthIssuer, cfg.AuthAudience, cfg.AuthAccessTokenTTL, cfg.AuthRefreshTokenTTL,
		emailSender, cfg.FrontendURL, cfg.AutoConfirmEmail, deps.Credits)
	authHandler.SetEnqueuer(enqueuer)
	authHandler.StartCleanup(ctx)
	oauthHandler := handler.NewOAuthHandler(database, rsaKey, signingKey,
		cfg.AuthIssuer, cfg.AuthAudience, cfg.AuthAccessTokenTTL, cfg.AuthRefreshTokenTTL,
		cfg.FrontendURL,
		cfg.OAuthGitHubClientID, cfg.OAuthGitHubClientSecret,
		cfg.OAuthGoogleClientID, cfg.OAuthGoogleClientSecret,
		cfg.OAuthXClientID, cfg.OAuthXClientSecret,
		deps.Credits)
	oauthHandler.SetEnqueuer(enqueuer)
	apiKeyHandler := handler.NewAPIKeyHandler(database, apiKeyCache, cacheManager)
	usageHandler := handler.NewUsageHandler(database)
	auditHandler := handler.NewAuditHandler(database)
	generationHandler := handler.NewGenerationHandler(database)
	reportingHandler := handler.NewReportingHandler(database)
	proxyHandler := handler.NewProxyHandler(cacheManager, &proxy.CaptureTransport{Inner: sentryobs.WrapTransport(proxy.NewTransport())})

	ragRuntime, err := setupRAGRuntime(cfg, database, enqueuer, mcpHandler)
	if err != nil {
		return err
	}
	preContextCache := precontext.NewRedisCache(redisClient)
	memorySearchService := buildMemorySearchService(cfg, database, cacheManager)
	memoryToolService := buildMemoryToolService(cfg, database, cacheManager, enqueuer)
	mcpHandler.SetMemoryTools(memory.NewToolsFunc(memoryToolService))
	preContextBuilder := buildPreContextService(
		cfg, database, preContextCache, memorySearchService,
		ragRuntime.qd, ragRuntime.embedder, ragRuntime.reranker,
	)
	memoryHandler := handler.NewMemoryHandler(database, enqueuer, cacheManager, cfg)
	nangoWebhookHandler := handler.NewNangoWebhookHandler(database, cfg.NangoWebhooksSecret, sandboxEncKey, nangoClient, enqueuer)

	incomingWebhookHandler := handler.NewIncomingWebhookHandler(database, enqueuer)
	httpTriggerHandler := handler.NewHTTPTriggerHandler(database, enqueuer)

	var templateBuilder handler.TemplateBuildable
	if orchestrator != nil {
		templateBuilder = orchestrator
	}
	sandboxTemplateHandler := handler.NewSandboxTemplateHandler(database, templateBuilder, enqueuer)
	skillHandler := handler.NewSkillHandler(database, enqueuer)
	pluginHandler := handler.NewPluginHandler(database, enqueuer)

	var agentHandler *handler.AgentHandler
	if orchestrator != nil {
		agentHandler = handler.NewAgentHandler(database, orchestrator, runtimeCompileDeps, reg)
		agentHandler.SetEnqueuer(enqueuer)
		orgHandler.SetAgentSyncer(agentHandler)
		authHandler.SetAgentSyncer(agentHandler)
		oauthHandler.SetAgentSyncer(agentHandler)
	}
	uploadsHandler := buildUploadsHandler(cfg, database, sandboxEncKey)
	if uploadsHandler != nil {
		uploadsHandler.WithUsageEnqueuer(enqueuer)
	}
	if uploadsHandler != nil && deps.KMS != nil {
		uploadsHandler.WithImageGeneration(deps.KMS, reg, &http.Client{
			Transport: &proxy.CaptureTransport{Inner: proxy.NewTransport()},
			Timeout:   3 * time.Minute,
		})
	}
	if uploadsHandler != nil {
		mcpHandler.SetImageGenerationTools(uploadsHandler.RegisterImageGenerationMCPTools)
	}
	imageDescribeHandler := buildImageDescribeHandler(database, cfg, deps)
	if imageDescribeHandler != nil {
		imageDescribeHandler.WithUsageEnqueuer(enqueuer)
	}
	if imageDescribeHandler != nil && uploadsHandler != nil {
		imageDescribeHandler.WithAssetReader(uploadsHandler.AssetReader())
	}
	if imageDescribeHandler != nil && sandboxEncKey != nil {
		imageDescribeHandler.WithRuntimeSecretKey(sandboxEncKey)
	}

	billingHandler := handler.NewBillingHandler(database, deps.BillingRegistry, deps.Credits)
	subscriptionHandler := handler.NewSubscriptionHandler(database, deps.BillingRegistry, deps.Credits)
	dashboardHandler := handler.NewDashboardHandler(database, deps.Credits)
	slackChannelHandler := handler.NewSlackChannelHandler(database, nangoClient, enqueuer)
	channelHandler := handler.NewChannelHandler(database)
	teamHandler := handler.NewTeamHandler(database)
	runtimeStreamStore := runtimestream.NewStore(redisClient, cfg.RuntimeRedisStreamShardCount)
	sessionHandler := handler.NewSessionHandler(database, enqueuer).
		WithRuntimeStreamKey(sandboxEncKey).
		WithRuntimeDelivery(orchestrator, runtimeCompileDeps).
		WithPreContextBuilder(preContextBuilder)
	sessionHandler.WithRuntimeStreamStore(runtimeStreamStore)
	if uploadsHandler != nil && deps.KMS != nil {
		sessionHandler.WithTranscription(
			deps.KMS,
			uploadsHandler.AssetReader(),
			transcription.NewElevenLabsTranscriber(&http.Client{
				Transport: &proxy.CaptureTransport{Inner: proxy.NewTransport()},
				Timeout:   65 * time.Second,
			}, 60*time.Second),
			reg,
		)
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

	setupPublicRoutes(r, cfg, database, redisClient, providerHandler, integrationHandler, actionsCatalog, orgInviteHandler, plansHandler, nangoWebhookHandler, incomingWebhookHandler, nangoClient, sandboxEncKey, deps.KMS, uploadsHandler, imageDescribeHandler, canvasHandler, orchestrator, orchestratorMissing)
	runtimeIngressHandler := handler.NewRuntimeStreamIngressHandler(database, sandboxEncKey, runtimeStreamStore)
	r.Get("/internal/runtime-events/sandboxes/{sandboxID}/sessions/{sessionID}/ws", runtimeIngressHandler.HandleSessionWS)
	r.Get("/internal/runtime-events/sandboxes/{sandboxID}/ws", runtimeIngressHandler.HandleWS)
	r.Post("/internal/runtime-events/sandboxes/{sandboxID}/turn-state", runtimeIngressHandler.HandleTurnState)

	r.Post("/incoming/triggers/{triggerID}", httpTriggerHandler.Handle)
	setupAuthRoutes(r, ctx, cfg, rsaPub, authHandler, oauthHandler)
	systemTaskHandler := buildSystemTaskHandler(database, deps, redisClient)
	setupV1Routes(r, cfg, rsaPub, database, apiKeyCache, enqueuer, orgHandler, orgInviteHandler, brandHandler, teamHandler, usageHandler, auditHandler, reportingHandler, generationHandler, apiKeyHandler, billingHandler, subscriptionHandler, dashboardHandler, slackChannelHandler, channelHandler, sessionHandler, memoryHandler, credHandler, tokenHandler, sandboxTemplateHandler, skillHandler, pluginHandler, databaseIntegrationHandler, ragRuntime.sourceHandler, ragRuntime.searchHandler, uploadsHandler, imageDescribeHandler, systemTaskHandler, agentHandler, canvasHandler, orchestrator, auditWriter)

	setupConnectRoutes(r, cfg, rsaPub, database, integrationHandler, connectionHandler, credHandler)
	setupProxyAndAuxRoutes(r, cfg, deps, signingKey, database, proxyHandler, auditWriter, generationWriter, ctr, enqueuer, runtimeCompileDeps)

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

	mcpSrv := setupMCPServer(ctx, cfg, signingKey, database, mcpHandler)

	<-ctx.Done()
	slog.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("server shutdown error", "error", err)
	}
	if err := mcpSrv.Shutdown(shutdownCtx); err != nil {
		slog.Error("mcp server shutdown error", "error", err)
	}

	auditWriter.Shutdown(shutdownCtx)
	generationWriter.Shutdown(shutdownCtx)
	if deps.ToolUsageWriter != nil {
		deps.ToolUsageWriter.Shutdown(shutdownCtx)
	}

	slog.Info("serve shutdown complete")
	return nil
}
