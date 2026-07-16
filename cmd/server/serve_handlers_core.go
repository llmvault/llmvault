package main

import (
	"context"
	"net/http"
	"time"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/bootstrap"
	"github.com/usehivy/hivy/internal/canvasartifact"
	"github.com/usehivy/hivy/internal/credentials"
	"github.com/usehivy/hivy/internal/email"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	sentryobs "github.com/usehivy/hivy/internal/observability/sentry"
	"github.com/usehivy/hivy/internal/proxy"
	"github.com/usehivy/hivy/internal/runtimestream"
	"github.com/usehivy/hivy/internal/sandbox"
	"github.com/usehivy/hivy/internal/sheets"
	"github.com/usehivy/hivy/internal/tasks"
	"github.com/usehivy/hivy/internal/webcrawl"
)

// serveHandlersCore holds the first half of the handlers/services built by
// buildServeHandlers, plus a couple of internal values (sheetsService,
// mcpHandler, runtimeCompileDeps) that buildServeHandlersRest needs to finish
// wiring the remaining handlers.
type serveHandlersCore struct {
	providerHandler            *handler.ProviderHandler
	integrationHandler         *handler.IntegrationHandler
	connectionHandler          *handler.ConnectionHandler
	orgHandler                 *handler.OrgHandler
	brandHandler               *handler.BrandHandler
	orgInviteHandler           *handler.OrgInviteHandler
	authHandler                *handler.AuthHandler
	oauthHandler               *handler.OAuthHandler
	apiKeyHandler              *handler.APIKeyHandler
	usageHandler               *handler.UsageHandler
	auditHandler               *handler.AuditHandler
	generationHandler          *handler.GenerationHandler
	reportingHandler           *handler.ReportingHandler
	proxyHandler               http.Handler
	sheetsHandler              *handler.SheetsHandler
	canvasHandler              *handler.CanvasHandler
	credHandler                *handler.CredentialHandler
	databaseIntegrationHandler *handler.DatabaseIntegrationHandler
	tokenHandler               *handler.TokenHandler
	mcpHandler                 *handler.MCPHandler
	auditWriter                *middleware.AuditWriter
	generationWriter           *middleware.GenerationWriter
	attributionCache           *middleware.AttributionCache
	runtimeCompileDeps         agentruntime.CompileDeps
	sheetsService              *sheets.Service
	runtimeStreamStore         *runtimestream.Store
}

// buildServeHandlersCore constructs the sheets/canvas/mcp/credential/auth
// handlers and other "core" services. It is the first half of the logic that
// used to live directly in runServe; buildServeHandlersRest builds the rest.
func buildServeHandlersCore(ctx context.Context, deps *bootstrap.Deps, enqueuer enqueue.TaskEnqueuer) (*serveHandlersCore, error) {
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

	auditWriter := middleware.NewAuditWriter(ctx, database, 10000)
	// One sheets service (publisher + import enqueuer) shared by the REST
	// handlers and the sheets MCP tool group.
	sheetsService := buildSheetsService(database, redisClient, enqueuer)
	sheetsHandler := buildSheetsHandler(cfg, database, redisClient, signingKey, sheetsService)
	runtimeStreamStore := runtimestream.NewStore(redisClient, cfg.RuntimeRedisStreamShardCount)
	canvasArtifactStore := buildCanvasArtifactStore(cfg)
	canvasArtifactService := canvasartifact.NewService(database, canvasArtifactStore)
	if redisClient != nil {
		canvasArtifactService.WithPublisher(canvasartifact.NewRuntimeNoticePublisher(runtimeStreamStore))
	}
	canvasHandler := handler.NewCanvasHandler(database, sandboxEncKey).WithArtifactService(canvasArtifactService)

	generationWriter := middleware.NewGenerationWriter(ctx, database, reg, 10000)
	if enqueuer != nil {
		// Durable fallback: spill billable generation rows to asynq rather than
		// dropping them on a DB blip, full buffer, or shutdown deadline.
		generationWriter.SetEnqueuer(enqueuer)
	}

	mcpHandler := handler.NewMCPHandler(database, signingKey, actionsCatalog, nangoClient, ctr)
	mcpHandler.SetKMS(deps.KMS)
	if deps.WebProvider != nil {
		mcpHandler.SetWebTools(webcrawl.NewWebToolsFunc(deps.WebProvider))
	}
	runtimeCompileDeps := agentruntime.CompileDeps{
		DB:         database,
		Picker:     credentials.NewPickerWithRegistry(database, reg),
		KMS:        deps.KMS,
		EncKey:     sandboxEncKey,
		SigningKey: signingKey,
		Cfg:        cfg,
		Nango:      nangoClient,
	}
	if orchestrator != nil {
		orchestrator.SetAgentRuntimeConfigPusher(func(ctx context.Context, sb *model.Sandbox, push sandbox.AgentRuntimeConfigPush) error {
			if push.Agent != nil {
				return agentruntime.PushAgentRuntimeConfigWithProxyTokenOptions(ctx, runtimeCompileDeps, push.Agent, sb, push.ProxyToken, push.RuntimeOptions)
			}
			return agentruntime.PushAgentRuntimeConfigForSandboxWithProxyTokenOptions(ctx, runtimeCompileDeps, sb, push.ProxyToken, push.RuntimeOptions)
		})
	}
	credHandler := handler.NewCredentialHandler(database, deps.KMS, cacheManager, ctr)
	databaseIntegrationHandler := handler.NewDatabaseIntegrationHandler(database, deps.KMS)
	tokenHandler := handler.NewTokenHandler(database, signingKey, cacheManager, ctr, actionsCatalog, cfg.MCPBaseURL, mcpHandler.ServerCache)
	providerHandler := handler.NewProviderHandler(reg, database)
	integrationHandler := handler.NewIntegrationHandler(database, nangoClient)
	connectionHandler := handler.NewConnectionHandler(database, nangoClient, actionsCatalog, enqueuer)
	orgHandler := handler.NewOrgHandler(database, enqueuer)
	brandHandler := handler.NewBrandHandler(database)
	// Handlers enqueue emails for async delivery when a queue is available;
	// without one (degraded/dev), send through the transport directly.
	var emailSender email.Sender
	if enqueuer != nil {
		emailSender = email.NewAsynqSender(enqueuer)
	} else {
		emailSender = email.NewTransport(emailTransportConfig(cfg))
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
	attributionCache := middleware.NewAttributionCache(50000, 20*time.Minute)
	proxyHandler := handler.NewProxyHandler(cacheManager, attributionCache, &proxy.CaptureTransport{Inner: sentryobs.WrapTransport(proxy.NewTransport())})

	return &serveHandlersCore{
		providerHandler:            providerHandler,
		integrationHandler:         integrationHandler,
		connectionHandler:          connectionHandler,
		orgHandler:                 orgHandler,
		brandHandler:               brandHandler,
		orgInviteHandler:           orgInviteHandler,
		authHandler:                authHandler,
		oauthHandler:               oauthHandler,
		apiKeyHandler:              apiKeyHandler,
		usageHandler:               usageHandler,
		auditHandler:               auditHandler,
		generationHandler:          generationHandler,
		reportingHandler:           reportingHandler,
		proxyHandler:               proxyHandler,
		attributionCache:           attributionCache,
		sheetsHandler:              sheetsHandler,
		canvasHandler:              canvasHandler,
		credHandler:                credHandler,
		databaseIntegrationHandler: databaseIntegrationHandler,
		tokenHandler:               tokenHandler,
		mcpHandler:                 mcpHandler,
		auditWriter:                auditWriter,
		generationWriter:           generationWriter,
		runtimeCompileDeps:         runtimeCompileDeps,
		sheetsService:              sheetsService,
		runtimeStreamStore:         runtimeStreamStore,
	}, nil
}
