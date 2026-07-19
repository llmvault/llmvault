package main

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/agentemail"
	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/agents"
	"github.com/usehivy/hivy/internal/apps"
	"github.com/usehivy/hivy/internal/bootstrap"
	"github.com/usehivy/hivy/internal/credentials"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/mcpservers"
	"github.com/usehivy/hivy/internal/memory"
	"github.com/usehivy/hivy/internal/precontext"
	"github.com/usehivy/hivy/internal/proxy"
	"github.com/usehivy/hivy/internal/registry"
	"github.com/usehivy/hivy/internal/runtimestream"
	"github.com/usehivy/hivy/internal/sheets"
	"github.com/usehivy/hivy/internal/skills"
	"github.com/usehivy/hivy/internal/transcription"
)

// serveHandlersRest holds the second half of the handlers/services built by
// buildServeHandlers, i.e. everything buildServeHandlersCore did not already
// build.
type serveHandlersRest struct {
	memoryHandler          *handler.MemoryHandler
	nangoWebhookHandler    *handler.NangoWebhookHandler
	incomingWebhookHandler *handler.IncomingWebhookHandler
	resendWebhookHandler   *handler.ResendWebhookHandler
	httpTriggerHandler     *handler.HTTPTriggerHandler
	sandboxTemplateHandler *handler.SandboxTemplateHandler
	agentHandler           *handler.AgentHandler
	appsInternalHandler    *handler.AppsInternalHandler
	appsHandler            *handler.AppsHandler
	uploadsHandler         *handler.UploadsHandler
	imageDescribeHandler   *handler.ImageDescribeHandler
	billingHandler         *handler.BillingHandler
	dashboardHandler       *handler.DashboardHandler
	teamHandler            *handler.TeamHandler
	runtimeStreamStore     *runtimestream.Store
	sessionHandler         *handler.SessionHandler
	transcriptionHandler   *handler.TranscriptionHandler
	mcpServerHandler       *handler.MCPServerHandler
	ragRuntime             *ragRuntime
}

// buildServeHandlersRest builds every handler not already built by
// buildServeHandlersCore. It relies on core.mcpHandler, core.sheetsService and
// core.runtimeCompileDeps to keep wiring identical to the original
// single-function version of this logic.
func buildServeHandlersRest(ctx context.Context, deps *bootstrap.Deps, enqueuer enqueue.TaskEnqueuer, core *serveHandlersCore) (*serveHandlersRest, error) {
	cfg := deps.Config
	database := deps.DB
	redisClient := deps.Redis
	cacheManager := deps.CacheManager
	reg := deps.Registry
	nangoClient := deps.NangoClient
	sandboxEncKey := deps.SandboxEncKey
	orchestrator := deps.Orchestrator
	mcpHandler := core.mcpHandler
	sheetsService := core.sheetsService
	runtimeCompileDeps := core.runtimeCompileDeps

	ragRuntime, err := setupRAGRuntime(cfg, database, enqueuer, mcpHandler, nangoClient, deps.ActionsCatalog, deps.WebProvider)
	if err != nil {
		return nil, err
	}
	preContextCache := precontext.NewRedisCache(redisClient)
	memorySearchService := buildMemorySearchService(cfg, database, cacheManager)
	memoryToolService := buildMemoryToolService(cfg, database, cacheManager, enqueuer)
	mcpHandler.SetMemoryTools(memory.NewToolsFunc(memoryToolService))        //nolint:contextcheck // tool handlers receive their own request context from the MCP server at call time.
	mcpHandler.SetSkillTools(skills.NewToolsFunc(database, cfg.FrontendURL)) //nolint:contextcheck // tool handlers receive their own request context from the MCP server at call time.
	// Assignable agent models: the full text-output catalog minus the scribe
	// transcription model. Used verbatim as the strict `model` enum in the
	// agent-builder tools; per-org credential availability is enforced at call
	// time by ValidateModel.
	agentBuilderModels := func() []string {
		provs := reg.AllProviders()
		provIDs := make([]string, 0, len(provs))
		for _, p := range provs {
			provIDs = append(provIDs, p.ID)
		}
		seen := map[string]bool{}
		var out []string
		for _, routed := range reg.CanonicalModelsForProviders(provIDs) {
			if !registry.ModelSupportsTextOutput(routed.Model) {
				continue
			}
			if routed.ID == "scribe-v2" { // the scribe transcription model
				continue
			}
			if seen[routed.ID] {
				continue
			}
			seen[routed.ID] = true
			out = append(out, routed.ID)
		}
		sort.Strings(out)
		return out
	}()
	agentBuilderDeps := agents.Deps{
		DB:           database,
		DefaultModel: agentruntime.DefaultAgentModel,
		Models:       agentBuilderModels,
		ValidateModel: func(ctx context.Context, orgID uuid.UUID, modelID string) error {
			if strings.TrimSpace(modelID) == "" {
				return fmt.Errorf("model is required")
			}
			if m, ok := reg.CanonicalModel(modelID); ok && !registry.ModelSupportsTextOutput(m.Model) {
				return fmt.Errorf("model %q does not support text output", modelID)
			}
			_, err := credentials.ResolveForModel(ctx, database, reg, orgID, modelID)
			return err
		},
	}
	mcpHandler.SetAgentBuilderTools(agents.NewToolsFunc(agentBuilderDeps, cfg.FrontendURL)) //nolint:contextcheck // tool handlers receive their own request context from the MCP server at call time.
	mcpHandler.SetSheetTools(sheets.NewToolsFunc(sheetsService))                            //nolint:contextcheck // tool handlers receive their own request context from the MCP server at call time.
	mcpHandler.SetEmailTools(agentemail.NewToolsFunc(database, enqueuer))                   //nolint:contextcheck // tool handlers receive their own request context from the MCP server at call time.
	preContextBuilder := buildPreContextService(
		cfg, database, preContextCache, memorySearchService,
		ragRuntime.qd, ragRuntime.embedder, ragRuntime.reranker,
	)
	memoryHandler := handler.NewMemoryHandler(database, enqueuer, cacheManager, cfg)
	nangoWebhookHandler := handler.NewNangoWebhookHandler(database, cfg.NangoWebhooksSecret, sandboxEncKey, nangoClient, enqueuer)

	incomingWebhookHandler := handler.NewIncomingWebhookHandler(database, enqueuer)
	resendWebhookHandler := handler.NewResendWebhookHandler(database, cfg.ResendWebhookSecret, cfg.AgentInboxDomain, enqueuer)
	httpTriggerHandler := handler.NewHTTPTriggerHandler(database, enqueuer)

	var templateBuilder handler.TemplateBuildable
	if orchestrator != nil {
		templateBuilder = orchestrator
	}
	sandboxTemplateHandler := handler.NewSandboxTemplateHandler(database, templateBuilder, enqueuer)

	var agentHandler *handler.AgentHandler
	if orchestrator != nil {
		agentHandler = handler.NewAgentHandler(database, orchestrator, runtimeCompileDeps, reg)
		agentHandler.SetEnqueuer(enqueuer)
	}
	appsInternalHandler := buildAppsInternalHandler(cfg, database, sheetsService, sandboxEncKey, redisClient)
	var appsHandler *handler.AppsHandler
	appsService := buildAppsService(cfg, database, sandboxEncKey, orchestrator, sheetsService, deps.RSAKey)
	if appsService != nil {
		appsHandler = handler.NewAppsHandler(database, appsService, deps.RSAKey)
		mcpHandler.SetAppsTools(apps.NewToolsFunc(appsService)) //nolint:contextcheck // tool handlers receive their own request context from the MCP server at call time.
	}
	uploadsHandler := buildUploadsHandler(cfg, database, sandboxEncKey)
	if uploadsHandler != nil {
		uploadsHandler.WithUsageEnqueuer(enqueuer)
		if appsService != nil {
			// Enables the preview-env side channel (apps_preview_env.go).
			uploadsHandler.WithAppsService(appsService)
		}
	}
	if uploadsHandler != nil && deps.KMS != nil {
		uploadsHandler.WithImageGeneration(deps.KMS, reg, &http.Client{
			Transport: &proxy.CaptureTransport{Inner: proxy.NewTransport()},
			Timeout:   3 * time.Minute,
		})
	}
	if uploadsHandler != nil {
		mcpHandler.SetImageGenerationTools(uploadsHandler.RegisterImageGenerationMCPTools)
		mcpHandler.SetDriveTools(uploadsHandler.RegisterDriveMCPTools)
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

	billingHandler := handler.NewBillingHandler(database, deps.Purchases, deps.Credits)
	dashboardHandler := handler.NewDashboardHandler(database, deps.Credits)
	slackResourceRouteValidator := handler.NewSlackResourceRouteValidator(database, nangoClient)
	teamHandler := handler.NewTeamHandler(
		database,
		handler.WithTeamEnvEncryptionKey(sandboxEncKey),
		handler.WithExternalResourceRouteValidator(slackResourceRouteValidator),
	)
	mcpOAuthCallbackURL := strings.TrimSpace(cfg.MCPOAuthCallbackURL)
	if mcpOAuthCallbackURL == "" {
		mcpOAuthCallbackURL = strings.TrimRight(cfg.APIWebhookBaseURL, "/") + "/v1/mcp-servers/oauth/callback"
	}
	mcpServerService := mcpservers.NewService(database, sandboxEncKey, mcpOAuthCallbackURL)
	mcpServerHandler := handler.NewMCPServerHandler(database, mcpServerService, cfg.FrontendURL)
	runtimeStreamStore := core.runtimeStreamStore
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
	// Org-scoped transcription (settings surfaces with no session) reuses the
	// same ElevenLabs stack as session transcription.
	var transcriptionHandler *handler.TranscriptionHandler
	if deps.KMS != nil {
		transcriptionHandler = handler.NewTranscriptionHandler(
			database,
			enqueuer,
			deps.KMS,
			transcription.NewElevenLabsTranscriber(&http.Client{
				Transport: &proxy.CaptureTransport{Inner: proxy.NewTransport()},
				Timeout:   65 * time.Second,
			}, 60*time.Second),
			reg,
		)
	}

	return &serveHandlersRest{
		memoryHandler:          memoryHandler,
		nangoWebhookHandler:    nangoWebhookHandler,
		incomingWebhookHandler: incomingWebhookHandler,
		resendWebhookHandler:   resendWebhookHandler,
		httpTriggerHandler:     httpTriggerHandler,
		sandboxTemplateHandler: sandboxTemplateHandler,
		agentHandler:           agentHandler,
		appsInternalHandler:    appsInternalHandler,
		appsHandler:            appsHandler,
		uploadsHandler:         uploadsHandler,
		imageDescribeHandler:   imageDescribeHandler,
		billingHandler:         billingHandler,
		dashboardHandler:       dashboardHandler,
		teamHandler:            teamHandler,
		runtimeStreamStore:     runtimeStreamStore,
		sessionHandler:         sessionHandler,
		transcriptionHandler:   transcriptionHandler,
		mcpServerHandler:       mcpServerHandler,
		ragRuntime:             ragRuntime,
	}, nil
}
