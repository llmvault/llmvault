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
	TypeSessionName               = "session:name"
	TypeAgentProxyTokenRefresh    = "agent:proxy_token_refresh"
	TypeAgentGitHubResourcesClone = "agent:github_resources_clone"
	TypeAgentScheduleDeliver      = "agent_schedule:deliver"
	TypeOrgHivyAgentProvision     = "org:hivy_agent_provision"
	TypePluginInstallSync         = "plugin:install_sync"
	TypeMemoryEmbed               = "memory:embed"
	TypeSandboxWarmPoolReconcile  = "sandbox:warm_pool_reconcile"
	TypeSandboxWarmSlotCheck      = "sandbox:warm_slot_check"

	// Periodic tasks (scheduled by the worker)
	TypeTokenCleanup         = "periodic:token_cleanup"
	TypeSandboxResourceCheck = "periodic:sandbox_resource_check"
	TypeSandboxReap          = "periodic:sandbox_reap"
	TypeCreditsExpire        = "periodic:credits_expire"
	TypeBillingBatchProcess  = "periodic:billing_batch_process"
	TypeBillingRenewSweep    = "periodic:billing_renew_sweep"
	TypeAgentScheduleScan    = "periodic:agent_schedule_scan"

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
