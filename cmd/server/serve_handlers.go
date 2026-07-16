package main

import (
	"context"
	"net/http"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/bootstrap"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/runtimestream"
)

// serveHandlers bundles every handler/service constructed by
// buildServeHandlers that is still needed once route registration begins.
type serveHandlers struct {
	providerHandler            *handler.ProviderHandler
	integrationHandler         *handler.IntegrationHandler
	connectionHandler          *handler.ConnectionHandler
	orgHandler                 *handler.OrgHandler
	brandHandler               *handler.BrandHandler
	plansHandler               *handler.PlansHandler
	orgInviteHandler           *handler.OrgInviteHandler
	authHandler                *handler.AuthHandler
	oauthHandler               *handler.OAuthHandler
	apiKeyHandler              *handler.APIKeyHandler
	usageHandler               *handler.UsageHandler
	auditHandler               *handler.AuditHandler
	generationHandler          *handler.GenerationHandler
	reportingHandler           *handler.ReportingHandler
	proxyHandler               http.Handler
	memoryHandler              *handler.MemoryHandler
	nangoWebhookHandler        *handler.NangoWebhookHandler
	incomingWebhookHandler     *handler.IncomingWebhookHandler
	httpTriggerHandler         *handler.HTTPTriggerHandler
	sandboxTemplateHandler     *handler.SandboxTemplateHandler
	agentHandler               *handler.AgentHandler
	appsInternalHandler        *handler.AppsInternalHandler
	appsHandler                *handler.AppsHandler
	uploadsHandler             *handler.UploadsHandler
	imageDescribeHandler       *handler.ImageDescribeHandler
	billingHandler             *handler.BillingHandler
	subscriptionHandler        *handler.SubscriptionHandler
	dashboardHandler           *handler.DashboardHandler
	slackChannelHandler        *handler.SlackChannelHandler
	channelHandler             *handler.ChannelHandler
	teamHandler                *handler.TeamHandler
	runtimeStreamStore         *runtimestream.Store
	sessionHandler             *handler.SessionHandler
	transcriptionHandler       *handler.TranscriptionHandler
	mcpServerHandler           *handler.MCPServerHandler
	credHandler                *handler.CredentialHandler
	databaseIntegrationHandler *handler.DatabaseIntegrationHandler
	tokenHandler               *handler.TokenHandler
	sheetsHandler              *handler.SheetsHandler
	canvasHandler              *handler.CanvasHandler
	mcpHandler                 *handler.MCPHandler
	ragRuntime                 *ragRuntime
	auditWriter                *middleware.AuditWriter
	generationWriter           *middleware.GenerationWriter
	attributionCache           *middleware.AttributionCache
	runtimeCompileDeps         agentruntime.CompileDeps
}

// buildServeHandlers constructs every handler and supporting service used by
// runServe. It is split into buildServeHandlersCore and buildServeHandlersRest
// purely to keep file sizes manageable; the construction order and logic are
// unchanged from the original single-function version of this code.
func buildServeHandlers(ctx context.Context, deps *bootstrap.Deps, enqueuer enqueue.TaskEnqueuer) (*serveHandlers, error) {
	core, err := buildServeHandlersCore(ctx, deps, enqueuer)
	if err != nil {
		return nil, err
	}
	rest, err := buildServeHandlersRest(ctx, deps, enqueuer, core)
	if err != nil {
		return nil, err
	}

	return &serveHandlers{
		providerHandler:            core.providerHandler,
		integrationHandler:         core.integrationHandler,
		connectionHandler:          core.connectionHandler,
		orgHandler:                 core.orgHandler,
		brandHandler:               core.brandHandler,
		plansHandler:               core.plansHandler,
		orgInviteHandler:           core.orgInviteHandler,
		authHandler:                core.authHandler,
		oauthHandler:               core.oauthHandler,
		apiKeyHandler:              core.apiKeyHandler,
		usageHandler:               core.usageHandler,
		auditHandler:               core.auditHandler,
		generationHandler:          core.generationHandler,
		reportingHandler:           core.reportingHandler,
		proxyHandler:               core.proxyHandler,
		memoryHandler:              rest.memoryHandler,
		nangoWebhookHandler:        rest.nangoWebhookHandler,
		incomingWebhookHandler:     rest.incomingWebhookHandler,
		httpTriggerHandler:         rest.httpTriggerHandler,
		sandboxTemplateHandler:     rest.sandboxTemplateHandler,
		agentHandler:               rest.agentHandler,
		appsInternalHandler:        rest.appsInternalHandler,
		appsHandler:                rest.appsHandler,
		uploadsHandler:             rest.uploadsHandler,
		imageDescribeHandler:       rest.imageDescribeHandler,
		billingHandler:             rest.billingHandler,
		subscriptionHandler:        rest.subscriptionHandler,
		dashboardHandler:           rest.dashboardHandler,
		slackChannelHandler:        rest.slackChannelHandler,
		channelHandler:             rest.channelHandler,
		teamHandler:                rest.teamHandler,
		runtimeStreamStore:         rest.runtimeStreamStore,
		sessionHandler:             rest.sessionHandler,
		transcriptionHandler:       rest.transcriptionHandler,
		mcpServerHandler:           rest.mcpServerHandler,
		credHandler:                core.credHandler,
		databaseIntegrationHandler: core.databaseIntegrationHandler,
		tokenHandler:               core.tokenHandler,
		sheetsHandler:              core.sheetsHandler,
		canvasHandler:              core.canvasHandler,
		mcpHandler:                 core.mcpHandler,
		ragRuntime:                 rest.ragRuntime,
		auditWriter:                core.auditWriter,
		generationWriter:           core.generationWriter,
		attributionCache:           core.attributionCache,
		runtimeCompileDeps:         core.runtimeCompileDeps,
	}, nil
}
