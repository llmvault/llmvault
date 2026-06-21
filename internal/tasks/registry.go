package tasks

import (
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/billing"
	"github.com/usehivy/hivy/internal/billing/subscription"
	"github.com/usehivy/hivy/internal/cache"
	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/nango"
	"github.com/usehivy/hivy/internal/precontext"
	"github.com/usehivy/hivy/internal/rag/scheduler"
	ragtasks "github.com/usehivy/hivy/internal/rag/tasks"
	"github.com/usehivy/hivy/internal/sandbox"
	"github.com/usehivy/hivy/internal/skills"
	"github.com/usehivy/hivy/internal/storage"
)

// WorkerDeps holds the dependencies needed by task handlers.
type WorkerDeps struct {
	DB                *gorm.DB
	Orchestrator      *sandbox.Orchestrator   // nil if sandbox not configured
	EncKey            *crypto.SymmetricKey    // nil if not configured
	EmailSend         EmailSenderFunc         // nil if email not configured
	EmailSendTemplate EmailTemplateSenderFunc // nil if template email not configured
	SkillFetcher      *skills.GitFetcher      // nil disables git skill hydration
	NangoClient       *nango.Client           // nil disables deterministic enrichment
	CacheManager      *cache.Manager          // nil disables tasks that need credential decryption
	Credits           *billing.CreditsService // required for billing-token-spend deduction
	Subscriptions     *subscription.Service   // required for renewal worker
	Enqueuer          enqueue.TaskEnqueuer    // required for enqueuing sub-tasks
	PreContextCache   precontext.Cache        // nil disables agent pre-context cache invalidation
	AgentCompile      agentruntime.CompileDeps
	OrgAgentSyncer    OrgHivyAgentSyncer
	CanvasSyncer      CanvasOrgSyncer
	S3Client          *storage.S3Client

	Rag          *ragtasks.Deps
	RagScheduler *scheduler.Deps
}

// NewServeMux creates an Asynq ServeMux with all task handlers registered.
func NewServeMux(deps *WorkerDeps) *asynq.ServeMux {
	mux := asynq.NewServeMux()

	mux.HandleFunc(TypeAPIKeyUpdate, NewAPIKeyHandler(deps.DB).Handle)
	mux.HandleFunc(TypeAuditWrite, NewAuditHandler(deps.DB).Handle)
	mux.HandleFunc(TypeGenerationWrite, NewGenerationHandler(deps.DB).Handle)

	mux.HandleFunc(TypeWebhookForward, NewWebhookForwardHandler(deps.EncKey).Handle)

	if deps.EmailSend != nil {
		mux.HandleFunc(TypeEmailSend, NewEmailSendHandler(deps.EmailSend).Handle)
	}
	if deps.EmailSendTemplate != nil {
		mux.HandleFunc(TypeEmailSendTemplate, NewEmailSendTemplateHandler(deps.EmailSendTemplate).Handle)
	}

	mux.HandleFunc(TypeTokenCleanup, NewTokenCleanupHandler(deps.DB).Handle)

	if deps.Orchestrator != nil {
		mux.HandleFunc(TypeSandboxHealthCheck, NewSandboxHealthCheckHandler(deps.Orchestrator).Handle)
		mux.HandleFunc(TypeSandboxResourceCheck, NewSandboxResourceCheckHandler(deps.Orchestrator).Handle)
		mux.HandleFunc(TypeSandboxLifecycle, NewSandboxLifecycleHandler(deps.Orchestrator).Handle)
		mux.HandleFunc(TypeSandboxReap, NewSandboxReapHandler(deps.Orchestrator).Handle)
		mux.HandleFunc(TypeAgentSandboxRetire, NewAgentSandboxRetireHandler(deps.DB, deps.Orchestrator).Handle)
		mux.HandleFunc(TypeSandboxWarmPoolReconcile, NewSandboxWarmPoolReconcileHandler(deps.Orchestrator, deps.Enqueuer).Handle)
		mux.HandleFunc(TypeSandboxWarmSlotCheck, NewSandboxWarmSlotCheckHandler(deps.Orchestrator, deps.Enqueuer).Handle)
	}

	// Agent cleanup works with or without sandbox orchestration.
	mux.HandleFunc(TypeAgentCleanup, NewAgentCleanupHandler(deps.DB, deps.Orchestrator).Handle)

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
		mux.HandleFunc(TypeCreditsExpire, NewCreditsExpireHandler(deps.Credits).Handle)
	}

	// Subscription renewal worker. Sweep runs hourly, finds due subs, and
	// enqueues per-sub renewal tasks; per-sub tasks call ChargeAuthorization
	// against the saved payment method.
	if deps.Subscriptions != nil && deps.Enqueuer != nil {
		mux.HandleFunc(TypeBillingRenewSweep, NewBillingRenewSweepHandler(deps.DB, deps.Enqueuer).Handle)
		mux.HandleFunc(TypeBillingRenewSubscription, NewBillingRenewSubscriptionHandler(deps.Subscriptions).Handle)
	}

	// Conversation naming (async title generation from the first message).
	// Requires the cache manager for credential decryption.
	if deps.CacheManager != nil {
		if handler := NewSessionNameHandler(deps.DB, deps.CacheManager); handler != nil {
			mux.HandleFunc(TypeSessionName, handler.Handle)
		}
	}

	mux.HandleFunc(TypeCanvasOrgSync, NewCanvasOrgSyncHandler(deps.CanvasSyncer).Handle)
	if deps.Orchestrator != nil && deps.S3Client != nil && deps.AgentCompile.EncKey != nil && deps.AgentCompile.KMS != nil {
		mux.HandleFunc(TypeAgentSandboxUpgrade,
			NewAgentSandboxUpgradeHandler(deps.DB, deps.Orchestrator, deps.S3Client, deps.AgentCompile, deps.Enqueuer).Handle)
		if AgentSandboxAutoUpgradeEnabled {
			mux.HandleFunc(TypeAgentSandboxAutoUpgrade,
				NewAgentSandboxAutoUpgradeHandler(deps.DB, deps.AgentCompile, deps.Enqueuer).Handle)
		}
	}

	if deps.Orchestrator != nil && deps.AgentCompile.EncKey != nil && deps.Enqueuer != nil {
		mux.HandleFunc(TypeAgentProxyTokenRefresh,
			NewAgentProxyTokenRefreshHandler(deps.DB, deps.Orchestrator, deps.AgentCompile, deps.Enqueuer).Handle)
	}
	if deps.Orchestrator != nil && deps.AgentCompile.EncKey != nil {
		if deps.OrgAgentSyncer != nil {
			mux.HandleFunc(TypeOrgHivyAgentProvision,
				NewOrgHivyAgentProvisionHandler(deps.OrgAgentSyncer).Handle)
		}
		mux.HandleFunc(TypeSessionMessageDeliver,
			NewSessionMessageDeliverHandler(deps.DB, deps.Orchestrator, deps.AgentCompile, deps.Enqueuer).Handle)
		triggerHandler := NewAgentTriggerDispatchHandler(deps.DB, deps.Orchestrator, deps.AgentCompile, deps.Enqueuer)
		triggerHandler.nangoClient = deps.NangoClient
		mux.HandleFunc(TypeAgentTriggerDispatch, triggerHandler.Handle)
		mux.HandleFunc(TypeAgentScheduleDeliver,
			NewAgentScheduleDeliverHandler(deps.DB, deps.Orchestrator, deps.AgentCompile, deps.Enqueuer).Handle)
		mux.HandleFunc(TypeAgentGitHubResourcesClone,
			NewAgentGitHubResourcesCloneHandler(deps.DB, deps.Orchestrator, deps.AgentCompile).Handle)
	}
	mux.HandleFunc(TypeAgentTriggerStoreDelivery, NewAgentTriggerStoreDeliveryHandler(deps.DB).Handle)
	if deps.Enqueuer != nil && deps.Orchestrator != nil && deps.AgentCompile.EncKey != nil {
		mux.HandleFunc(TypeAgentScheduleScan, NewAgentScheduleScanHandler(deps.DB, deps.Enqueuer).Handle)
	}

	if deps.Rag != nil {
		ragtasks.RegisterHandlers(mux, deps.Rag)
	}
	if deps.RagScheduler != nil {
		deps.RagScheduler.Register(mux)
	}

	return mux
}
