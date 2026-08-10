package tasks

import (
	"strings"
	"time"

	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/agentemail"
	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/billing"
	"github.com/usehivy/hivy/internal/cache"
	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/nango"
	"github.com/usehivy/hivy/internal/precontext"
	"github.com/usehivy/hivy/internal/rag/scheduler"
	ragtasks "github.com/usehivy/hivy/internal/rag/tasks"
	"github.com/usehivy/hivy/internal/sandbox"
	"github.com/usehivy/hivy/internal/sheets"
	"github.com/usehivy/hivy/internal/skills"
	"github.com/usehivy/hivy/internal/storage"
)

// WorkerDeps holds the dependencies needed by task handlers.
type WorkerDeps struct {
	DB                  *gorm.DB
	Orchestrator        *sandbox.Orchestrator   // nil if sandbox not configured
	EncKey              *crypto.SymmetricKey    // nil if not configured
	EmailSend           EmailSenderFunc         // nil if email not configured
	EmailSendTemplate   EmailTemplateSenderFunc // nil if template email not configured
	SkillFetcher        *skills.GitFetcher      // nil disables git skill hydration
	NangoClient         *nango.Client           // nil disables deterministic enrichment
	CacheManager        *cache.Manager          // nil disables tasks that need credential decryption
	Credits             *billing.CreditsService // required for billing-token-spend deduction
	Enqueuer            enqueue.TaskEnqueuer    // required for enqueuing sub-tasks
	PreContextCache     precontext.Cache        // nil disables agent pre-context cache invalidation
	PreContextBuilder   precontext.Builder      // nil disables runtime pre-context injection
	AgentCompile        agentruntime.CompileDeps
	OrgAgentEnsurer     OrgHivyAgentEnsurer
	SlackMediaEnricher  SlackMediaEnricher
	Storage             storage.Reader               // nil disables the sheet CSV import worker
	SheetEvents         sheets.EventPublisher        // nil disables sheet realtime events from workers
	UsageNotices        UsageNoticePublisher         // nil disables usage.updated notices from the model-usage worker
	SessionEventNotices SessionEventsNoticePublisher // nil disables session.events.appended notices from trigger/schedule injection
	AgentEmail          *agentemail.Client           // nil disables Resend agent inbox processing
	AgentInboxDomain    string                       // required to send from agent inboxes
	SandboxIdleTimeout  time.Duration                // configured idle cutoff for sandbox auto-sleep

	Rag          *ragtasks.Deps
	RagScheduler *scheduler.Deps
}

// NewServeMux creates an Asynq ServeMux with all task handlers registered.
func NewServeMux(deps *WorkerDeps) *asynq.ServeMux {
	mux := asynq.NewServeMux()

	mux.HandleFunc(TypeAPIKeyUpdate, NewAPIKeyHandler(deps.DB).Handle)
	mux.HandleFunc(TypeAuditWrite, NewAuditHandler(deps.DB).Handle)
	mux.HandleFunc(TypeGenerationWrite, NewGenerationHandler(deps.DB).Handle)
	mux.HandleFunc(TypeModelUsageWrite, NewModelUsageHandler(deps.DB, deps.UsageNotices).Handle)

	mux.HandleFunc(TypeWebhookForward, NewWebhookForwardHandler(deps.EncKey).Handle)

	if deps.EmailSend != nil {
		mux.HandleFunc(TypeEmailSend, NewEmailSendHandler(deps.EmailSend).Handle)
	}
	if deps.EmailSendTemplate != nil {
		mux.HandleFunc(TypeEmailSendTemplate, NewEmailSendTemplateHandler(deps.EmailSendTemplate).Handle)
	}

	mux.HandleFunc(TypeTokenCleanup, NewTokenCleanupHandler(deps.DB).Handle)
	mux.HandleFunc(TypeSessionTurnWatchdog, NewSessionTurnWatchdogHandler(deps.DB).Handle)

	if deps.Orchestrator != nil {
		mux.HandleFunc(TypeSandboxResourceCheck, NewSandboxResourceCheckHandler(deps.Orchestrator).Handle)
		mux.HandleFunc(TypeSandboxReap, NewSandboxReapHandler(deps.Orchestrator).Handle)
		mux.HandleFunc(TypeSandboxWarmPoolReconcile, NewSandboxWarmPoolReconcileHandler(deps.Orchestrator, deps.Enqueuer).Handle)
		mux.HandleFunc(TypeSandboxWarmSlotCheck, NewSandboxWarmSlotCheckHandler(deps.Orchestrator, deps.Enqueuer).Handle)
		if deps.Enqueuer != nil {
			mux.HandleFunc(TypeSandboxAutoSleep, NewSandboxAutoSleepHandler(deps.DB, deps.Enqueuer, deps.SandboxIdleTimeout).Handle)
			mux.HandleFunc(TypeSandboxSleep, NewSandboxSleepHandler(deps.DB, deps.Orchestrator, deps.SandboxIdleTimeout).Handle)
		}
		mux.HandleFunc(TypeSandboxReconcile, NewSandboxReconcileHandler(deps.DB, deps.Orchestrator).Handle)
		// Tears down a single session's sandbox after the session is archived.
		mux.HandleFunc(TypeSandboxDelete, NewSandboxDeleteHandler(deps.DB, deps.Orchestrator).Handle)
	}

	// Flips a slept sandbox back to 'running' when its runtime websocket reconnects.
	mux.HandleFunc(TypeSandboxMarkRunning, NewSandboxMarkRunningHandler(deps.DB).Handle)

	if deps.Orchestrator != nil {
		handler := NewSandboxTemplateBuildHandler(deps.DB, deps.Orchestrator)
		mux.HandleFunc(TypeSandboxTemplateBuild, handler.Handle)
		mux.HandleFunc(TypeSandboxTemplateRetryBuild, handler.HandleRetry)
	}

	if deps.SkillFetcher != nil {
		mux.HandleFunc(TypeSkillHydrate, NewSkillHydrateHandler(deps.DB, deps.SkillFetcher).Handle)
	}

	if deps.Credits != nil {
		mux.HandleFunc(TypeBillingBatchProcess, NewBillingBatchProcessHandler(deps.DB, deps.Credits).Handle)
		mux.HandleFunc(TypeSandboxBillingProcess, NewSandboxBillingProcessHandler(deps.DB, deps.UsageNotices).Handle)
	}

	if deps.CacheManager != nil {
		mux.HandleFunc(TypeGenerationReconcile, NewGenerationReconcileHandler(deps.DB, deps.CacheManager).Handle)
	}

	// Conversation naming (async title generation from the first message).
	// Requires the cache manager for credential decryption.
	if deps.CacheManager != nil {
		if handler := NewSessionNameHandler(deps.DB, deps.CacheManager); handler != nil {
			mux.HandleFunc(TypeSessionName, handler.Handle)
		}
		mux.HandleFunc(TypeMemoryEmbed, NewMemoryEmbedHandler(deps.DB, deps.CacheManager, memoryEmbeddingConfigFromDeps(deps)).Handle)
		mux.HandleFunc(TypeObservationEmbed,
			NewObservationEmbedHandler(deps.DB, deps.CacheManager, memoryEmbeddingConfigFromDeps(deps)).Handle)
		mux.HandleFunc(TypeMemoryObservationExpire,
			NewMemoryObservationExpireHandler(deps.DB, deps.CacheManager, memoryEmbeddingConfigFromDeps(deps)).Handle)
		if deps.Enqueuer != nil {
			mux.HandleFunc(TypeSessionReflection,
				NewSessionReflectionHandler(deps.DB, deps.CacheManager, deps.Enqueuer, memoryEmbeddingConfigFromDeps(deps)).Handle)
			mux.HandleFunc(TypeMemoryConsolidate,
				NewMemoryConsolidationHandler(deps.DB, deps.CacheManager, deps.Enqueuer, memoryEmbeddingConfigFromDeps(deps)).Handle)
		}
	}
	if deps.Enqueuer != nil {
		mux.HandleFunc(TypeSessionReflectionScan, NewSessionReflectionScanHandler(deps.DB, deps.Enqueuer).Handle)
		mux.HandleFunc(TypeMemoryConsolidationSweep, NewMemoryConsolidationSweepHandler(deps.DB, deps.Enqueuer).Handle)
		mux.HandleFunc(TypeSessionReviewBatchFlush, NewSessionReviewBatchFlushHandler(deps.DB, deps.Enqueuer).Handle)
	}

	if deps.Orchestrator != nil && deps.AgentCompile.EncKey != nil && deps.Enqueuer != nil {
		mux.HandleFunc(TypeAgentProxyTokenRefresh,
			NewAgentProxyTokenRefreshHandler(deps.DB, deps.Orchestrator, deps.AgentCompile, deps.Enqueuer).Handle)
	}
	if deps.Orchestrator != nil && deps.AgentCompile.EncKey != nil {
		if deps.OrgAgentEnsurer != nil {
			mux.HandleFunc(TypeOrgHivyAgentProvision,
				NewOrgHivyAgentProvisionHandler(deps.OrgAgentEnsurer).Handle)
		}
		mux.HandleFunc(TypeSessionMessageDeliver,
			NewSessionMessageDeliverHandler(deps.DB, deps.Orchestrator, deps.AgentCompile, deps.Enqueuer).Handle)
		mux.HandleFunc(TypeSlackAppMention,
			NewSlackAppMentionHandler(deps.DB, deps.Orchestrator, deps.AgentCompile, deps.Enqueuer, deps.NangoClient,
				WithSlackMediaEnricher(deps.SlackMediaEnricher), WithSlackRouterCache(deps.CacheManager)).Handle)
		mux.HandleFunc(TypeSlackReactionTrigger,
			NewSlackReactionTriggerHandler(deps.DB, deps.Orchestrator, deps.AgentCompile, deps.Enqueuer, deps.NangoClient,
				WithSlackMediaEnricher(deps.SlackMediaEnricher)).Handle)
		triggerHandler := NewAgentTriggerDispatchHandler(deps.DB, deps.Orchestrator, deps.AgentCompile, deps.Enqueuer)
		triggerHandler.nangoClient = deps.NangoClient
		triggerHandler.sessionEventNotices = deps.SessionEventNotices
		mux.HandleFunc(TypeAgentTriggerDispatch, triggerHandler.Handle)
		scheduleHandler := NewAgentScheduleDeliverHandler(deps.DB, deps.Orchestrator, deps.AgentCompile, deps.Enqueuer)
		scheduleHandler.sessionEventNotices = deps.SessionEventNotices
		mux.HandleFunc(TypeAgentScheduleDeliver, scheduleHandler.Handle)
		mux.HandleFunc(TypeAgentGitHubResourcesClone,
			NewAgentGitHubResourcesCloneHandler(deps.DB, deps.Orchestrator, deps.AgentCompile).Handle)
	}
	mux.HandleFunc(TypeAgentTriggerStoreDelivery, NewAgentTriggerStoreDeliveryHandler(deps.DB).Handle)
	if deps.AgentEmail != nil && deps.AgentEmail.Configured() && strings.TrimSpace(deps.AgentInboxDomain) != "" && deps.Enqueuer != nil {
		mux.HandleFunc(TypeAgentEmailReceive, NewAgentEmailReceiveHandler(deps.DB, deps.AgentEmail, deps.Enqueuer, deps.AgentInboxDomain).Handle)
		mux.HandleFunc(TypeAgentEmailSend, NewAgentEmailSendHandler(deps.DB, deps.AgentEmail, deps.AgentInboxDomain).Handle)
	}
	if deps.Enqueuer != nil && deps.Orchestrator != nil && deps.AgentCompile.EncKey != nil {
		mux.HandleFunc(TypeAgentScheduleScan, NewAgentScheduleScanHandler(deps.DB, deps.Enqueuer).Handle)
	}

	// CSV import needs S3 read access to stream the uploaded object.
	if deps.Storage != nil {
		mux.HandleFunc(TypeSheetCSVImport, NewSheetCSVImportHandler(deps.DB, deps.Storage, deps.SheetEvents).Handle)
	}

	if deps.Rag != nil {
		ragtasks.RegisterHandlers(mux, deps.Rag)
	}
	if deps.RagScheduler != nil {
		deps.RagScheduler.Register(mux)
	}

	return mux
}
