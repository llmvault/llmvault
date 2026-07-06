package tasks

// Task type constants for all Asynq tasks.
const (
	// On-demand tasks (enqueued by HTTP handlers / middleware)
	TypeWebhookForward            = "webhook:forward"
	TypeAuditWrite                = "audit:write"
	TypeGenerationWrite           = "generation:write"
	TypeModelUsageWrite           = "model_usage:write"
	TypeAPIKeyUpdate              = "apikey:update_last_used" // #nosec G101 -- task type identifier, not a credential
	TypeEmailSend                 = "email:send"
	TypeEmailSendTemplate         = "email:send_template"
	TypeAgentCleanup              = "agent:cleanup"
	TypeSandboxTemplateBuild      = "sandbox_template:build"
	TypeSandboxTemplateRetryBuild = "sandbox_template:retry"
	TypeSkillHydrate              = "skill:hydrate"
	TypeAgentTriggerDispatch      = "agent_trigger:dispatch"
	TypeAgentTriggerStoreDelivery = "agent_trigger:store_delivery"
	TypeSessionMessageDeliver     = "session:message_deliver"
	TypeSessionReflection         = "session:reflect"
	TypeSlackAppMention           = "slack:app_mention"
	TypeSlackReactionTrigger      = "slack:reaction_trigger"
	TypeSessionName               = "session:name"
	TypeAgentProxyTokenRefresh    = "agent:proxy_token_refresh"
	TypeAgentGitHubResourcesClone = "agent:github_resources_clone"
	TypeAgentScheduleDeliver      = "agent_schedule:deliver"
	TypeOrgHivyAgentProvision     = "org:hivy_agent_provision"
	TypeMemoryEmbed               = "memory:embed"
	TypeMemoryConsolidate         = "memory:consolidate"
	TypeObservationEmbed          = "observation:embed"
	TypeSandboxWarmPoolReconcile  = "sandbox:warm_pool_reconcile"
	TypeSandboxWarmSlotCheck      = "sandbox:warm_slot_check"
	TypeSandboxMarkRunning        = "sandbox:mark_running"
	TypeSandboxDelete             = "sandbox:delete"
	TypeChannelMemoriesDelete     = "channel:memories_delete"
	TypeSheetCSVImport            = "sheet:csv_import"

	// Periodic tasks (scheduled by the worker)
	TypeTokenCleanup          = "periodic:token_cleanup"
	TypeSandboxResourceCheck  = "periodic:sandbox_resource_check"
	TypeSandboxReap           = "periodic:sandbox_reap"
	TypeCreditsExpire         = "periodic:credits_expire"
	TypeBillingBatchProcess   = "periodic:billing_batch_process"
	TypeBillingRenewSweep     = "periodic:billing_renew_sweep"
	TypeAgentScheduleScan     = "periodic:agent_schedule_scan"
	TypeSessionReflectionScan = "periodic:session_reflection_scan"
	TypeSandboxAutoSleep      = "periodic:sandbox_auto_sleep"
	TypeSandboxReconcile      = "periodic:sandbox_reconcile"
	TypeSessionTurnWatchdog   = "periodic:session_turn_watchdog"
	// Sweep for reflection facts that missed their post-reflection
	// consolidation enqueue (stranded unconsolidated facts).
	TypeMemoryConsolidationSweep = "periodic:memory_consolidation_sweep"
	// Nightly archive of observations whose expires_at has passed.
	TypeMemoryObservationExpire = "periodic:memory_observation_expire"

	// On-demand task enqueued by the sweep for each due subscription.
	TypeBillingRenewSubscription = "billing:renew_subscription"
)

// Queue names with priority weights.
const (
	QueueCritical = "critical"
	QueueDefault  = "default"
	QueueBulk     = "bulk"
	QueuePeriodic = "periodic"
)
