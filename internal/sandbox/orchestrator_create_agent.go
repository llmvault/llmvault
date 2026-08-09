package sandbox

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/observability/correlation"
)

func (o *Orchestrator) CreateAgentSandboxWithRuntimeOptions(ctx context.Context, agent *model.Agent, secrets *agentruntime.StartupSecrets, runtimeOptions agentruntime.RuntimeConfigOptions) (result *model.Sandbox, resultErr error) {
	correlationValues := correlation.FromContext(ctx)
	if runtimeOptions.SessionID != uuid.Nil {
		correlationValues.SessionID = runtimeOptions.SessionID.String()
	}
	if runtimeOptions.ProvisioningAttemptID != uuid.Nil {
		correlationValues.ProvisioningAttemptID = runtimeOptions.ProvisioningAttemptID.String()
	}
	if traceID := strings.TrimSpace(runtimeOptions.TraceID); traceID != "" {
		correlationValues.TraceID = traceID
	}
	ctx = correlation.WithValues(ctx, correlationValues)
	totalStarted := time.Now()
	phaseStarted := totalStarted
	lastPhase := "start"
	logPhase := func(phase string, attrs ...any) {
		attrs = append(attrs,
			"total_ms", time.Since(totalStarted).Milliseconds(),
		)
		logging.LogPhase(ctx, "agent sandbox create phase", phase, phaseStarted, attrs...)
		phaseStarted = time.Now()
		lastPhase = phase
	}
	defer func() {
		if resultErr != nil {
			logging.FromContext(ctx).ErrorContext(ctx, "agent sandbox creation failed",
				"event", "agent sandbox create phase",
				"status", "error",
				"last_completed_phase", lastPhase,
				"duration_ms", time.Since(phaseStarted).Milliseconds(),
				"total_ms", time.Since(totalStarted).Milliseconds(),
				"error", resultErr,
			)
		}
	}()
	if agent == nil || agent.OrgID == nil {
		return nil, fmt.Errorf("CreateAgentSandbox: agent must have org_id")
	}
	if secrets == nil || secrets.ProxyToken == "" || secrets.ProxyTokenJTI == "" {
		return nil, fmt.Errorf("CreateAgentSandbox: proxy token is required")
	}
	startupProxyToken := &agentruntime.ProxyTokenResult{
		Token:     secrets.ProxyToken,
		JTI:       secrets.ProxyTokenJTI,
		ExpiresAt: secrets.ProxyExpires,
	}
	orgID := *agent.OrgID
	logPhase("start",
		"agent_id", agent.ID,
		"org_id", orgID,
		"requested_model", strings.TrimSpace(runtimeOptions.ModelID),
		"has_reasoning_effort", strings.TrimSpace(runtimeOptions.ReasoningEffort) != "",
	)
	exposedPorts, err := o.loadOrgSandboxExposedPorts(ctx, orgID)
	if err != nil {
		return nil, err
	}
	logPhase("load exposed ports", "agent_id", agent.ID, "org_id", orgID, "port_count", len(exposedPorts))

	gitIdentity, err := o.loadAgentGitIdentity(ctx, agent)
	if err != nil {
		return nil, fmt.Errorf("loading agent git identity: %w", err)
	}
	logPhase("load git identity", "agent_id", agent.ID, "org_id", orgID, "has_git_identity", gitIdentity != nil)

	runtimeSecret, err := generateRandomHex(32)
	if err != nil {
		return nil, fmt.Errorf("generating runtime secret: %w", err)
	}
	encryptedSecret, err := o.encKey.EncryptString(runtimeSecret)
	if err != nil {
		return nil, fmt.Errorf("encrypting runtime secret: %w", err)
	}
	logPhase("prepare runtime secret", "agent_id", agent.ID, "org_id", orgID)

	sandboxSize := model.NormalizeTemplateSize(agent.SandboxSize)
	templateRef := ""
	if agent.SandboxTemplateID != nil {
		var tmpl model.SandboxTemplate
		if err := o.db.WithContext(ctx).
			Where("id = ? AND (org_id = ? OR org_id IS NULL)", *agent.SandboxTemplateID, orgID).
			First(&tmpl).Error; err != nil {
			return nil, fmt.Errorf("loading sandbox template: %w", err)
		}
		if err := o.ensureTemplateProvider(&tmpl); err != nil {
			return nil, err
		}
		if tmpl.BuildStatus != "ready" || tmpl.ExternalID == nil || strings.TrimSpace(*tmpl.ExternalID) == "" {
			return nil, fmt.Errorf("sandbox template %s is not ready", tmpl.ID)
		}
		templateRef = strings.TrimSpace(*tmpl.ExternalID)
		if strings.TrimSpace(tmpl.Size) != "" {
			sandboxSize = model.NormalizeTemplateSize(tmpl.Size)
		}
	}
	resourceSpec, _ := model.TemplateSizeSpec(sandboxSize)
	sandboxImage := runtimeSandboxImageForProvider(o.provider.ID(), agent.SandboxImage)
	runtimeImage := AgentRuntimeImageRef(o.cfg, sandboxImage)
	warmProfile := AgentWarmPoolProfile(o.cfg, sandboxImage, sandboxSize)
	snapshotID := RuntimeTemplateRefForImageRef(o.cfg, runtimeImage, sandboxSize)
	if templateRef != "" {
		snapshotID = templateRef
	}
	logPhase("resolve template",
		"agent_id", agent.ID,
		"org_id", orgID,
		"sandbox_size", sandboxSize,
		"sandbox_image", sandboxImage,
		"runtime_image", runtimeImage,
		"has_template_ref", templateRef != "",
		"snapshot_id", snapshotID,
	)
	sb := model.Sandbox{
		OrgID:                  &orgID,
		AgentID:                &agent.ID,
		SandboxTemplateID:      agent.SandboxTemplateID,
		SnapshotID:             &snapshotID,
		ProviderID:             o.provider.ID(),
		EncryptedRuntimeSecret: encryptedSecret,
		Status:                 "creating",
		ExposedPorts:           model.SandboxExposedPortsInt64Array(exposedPorts),
	}
	if err := o.db.Create(&sb).Error; err != nil {
		return nil, fmt.Errorf("saving sandbox: %w", err)
	}
	ctx = correlation.WithSandboxID(ctx, sb.ID.String())
	logPhase("save sandbox row", "sandbox_id", sb.ID, "agent_id", agent.ID, "org_id", orgID)

	bugsinkDashboardURL := agentruntime.BugsinkDashboardBaseURL(ctx, o.db, orgID, *agent)
	glitchTipDashboardURL := agentruntime.GlitchTipDashboardBaseURL(ctx, o.db, orgID, *agent)
	envVars := agentSandboxEnvVars(ctx, o.db, o.cfg, runtimeSecret, &sb, orgID, agent, secrets, gitIdentity, bugsinkDashboardURL, glitchTipDashboardURL)
	if runtimeOptions.SessionID != uuid.Nil {
		envVars[agentruntime.AgentEnvSessionID] = runtimeOptions.SessionID.String()
	}
	if runtimeOptions.ProvisioningAttemptID != uuid.Nil {
		envVars[agentruntime.AgentEnvProvisioningAttemptID] = runtimeOptions.ProvisioningAttemptID.String()
	}
	if traceID := strings.TrimSpace(runtimeOptions.TraceID); traceID != "" {
		envVars[agentruntime.AgentEnvTraceID] = traceID
	}
	labels := map[string]string{
		"org_id":        orgID.String(),
		"sandbox_id":    sb.ID.String(),
		"agent_id":      agent.ID.String(),
		"harness":       "agent-sandbox",
		"sandbox_size":  sandboxSize,
		"sandbox_image": sandboxImage,
	}
	correlationValues.SandboxID = sb.ID.String()
	correlationValues.OrgID = orgID.String()
	correlationValues.AgentID = agent.ID.String()
	correlation.ApplyLabels(labels, correlationValues)
	logPhase("build provider request",
		"sandbox_id", sb.ID,
		"agent_id", agent.ID,
		"org_id", orgID,
		"env_key_count", len(envVars),
		"label_count", len(labels),
		"has_bugsink_dashboard_url", strings.TrimSpace(bugsinkDashboardURL) != "",
		"has_glitchtip_dashboard_url", strings.TrimSpace(glitchTipDashboardURL) != "",
	)

	configPush := AgentRuntimeConfigPush{Agent: agent, RuntimeOptions: runtimeOptions}
	if o.shouldTryAgentWarmPool(templateRef, exposedPorts) {
		claimed, err := o.tryClaimWarmRuntime(ctx, &sb, warmProfile, configPush)
		if err != nil {
			if delErr := o.db.Where("id = ?", sb.ID).Delete(&model.Sandbox{}).Error; delErr != nil {
				logging.FromContext(ctx).ErrorContext(ctx, "delete orphaned agent sandbox row after warm claim failure",
					"error", delErr, "sandbox_id", sb.ID)
			}
			return nil, err
		}
		if claimed {
			logging.FromContext(ctx).InfoContext(ctx, "agent sandbox claimed from warm pool",
				"sandbox_id", sb.ID, "external_id", sb.ExternalID, "agent_id", agent.ID,
				"sandbox_image", sandboxImage, "sandbox_size", sandboxSize)
			logPhase("complete",
				"sandbox_id", sb.ID,
				"external_id", sb.ExternalID,
				"agent_id", agent.ID,
				"org_id", orgID,
				"provisioning_path", "warm_pool",
			)
			return &sb, nil
		}
		logging.FromContext(ctx).InfoContext(ctx, "agent sandbox warm pool empty; creating directly",
			"sandbox_id", sb.ID, "agent_id", agent.ID, "sandbox_image", sandboxImage, "sandbox_size", sandboxSize)
	}
	logPhase("warm pool check",
		"sandbox_id", sb.ID,
		"agent_id", agent.ID,
		"org_id", orgID,
		"warm_pool_profile", warmProfile,
		"warm_pool_eligible", o.shouldTryAgentWarmPool(templateRef, exposedPorts),
	)

	info, err := o.provider.CreateSandbox(ctx, CreateSandboxOpts{
		Name:         buildAgentSandboxName(agent),
		TemplateRef:  snapshotID,
		EnvVars:      envVars,
		Labels:       labels,
		CPU:          resourceSpec.CPU,
		Memory:       resourceSpec.Memory,
		Disk:         resourceSpec.Disk,
		ExposedPorts: exposedPorts,
	})
	if err != nil {
		if delErr := o.db.Where("id = ?", sb.ID).Delete(&model.Sandbox{}).Error; delErr != nil {
			logging.FromContext(ctx).ErrorContext(ctx, "delete orphaned agent sandbox row after provider create failure",
				"error", delErr, "sandbox_id", sb.ID)
		}
		return nil, fmt.Errorf("provider create: %w", err)
	}
	logPhase("provider create",
		"sandbox_id", sb.ID,
		"external_id", info.ExternalID,
		"agent_id", agent.ID,
		"org_id", orgID,
		"provider_id", o.provider.ID(),
	)

	sandboxURL, err := o.provider.GetEndpoint(ctx, info.ExternalID, AgentSandboxPort)
	if err != nil {
		o.cleanupFailedSandbox(ctx, &sb, info.ExternalID, "get endpoint failed")
		return nil, fmt.Errorf("getting agent runtime endpoint: %w", err)
	}
	logPhase("get runtime endpoint",
		"sandbox_id", sb.ID,
		"external_id", info.ExternalID,
		"agent_id", agent.ID,
		"org_id", orgID,
		"runtime_port", AgentSandboxPort,
	)

	now := time.Now()
	expiresAt := now.Add(runtimeURLTTL)
	if err := o.db.Model(&sb).Updates(map[string]any{
		"external_id":            info.ExternalID,
		"runtime_url":            sandboxURL,
		"runtime_url_expires_at": expiresAt,
		"status":                 "running",
		"last_active_at":         now,
	}).Error; err != nil {
		o.cleanupFailedSandbox(ctx, &sb, info.ExternalID, fmt.Sprintf("updating sandbox row failed: %v", err))
		return nil, fmt.Errorf("updating sandbox: %w", err)
	}
	sb.ExternalID = info.ExternalID
	sb.RuntimeURL = sandboxURL
	sb.RuntimeURLExpiresAt = &expiresAt
	sb.Status = "running"
	sb.LastActiveAt = &now
	logPhase("persist runtime endpoint",
		"sandbox_id", sb.ID,
		"external_id", info.ExternalID,
		"agent_id", agent.ID,
		"org_id", orgID,
		"runtime_url_expires_at", expiresAt,
	)

	if err := o.waitForAgentRuntimeLive(ctx, &sb); err != nil {
		o.cleanupFailedSandbox(ctx, &sb, info.ExternalID, "agent runtime not live")
		return nil, fmt.Errorf("waiting for agent runtime: %w", err)
	}
	logPhase("wait runtime live", "sandbox_id", sb.ID, "external_id", info.ExternalID, "agent_id", agent.ID, "org_id", orgID)
	configPush.ProxyToken = startupProxyToken
	if err := o.pushAgentRuntimeConfig(ctx, &sb, "create", configPush); err != nil {
		o.cleanupFailedSandbox(ctx, &sb, info.ExternalID, "agent runtime config push failed")
		return nil, err
	}
	logPhase("push runtime config", "sandbox_id", sb.ID, "external_id", info.ExternalID, "agent_id", agent.ID, "org_id", orgID)

	idleTimeout := time.Duration(0)
	if o.cfg != nil {
		idleTimeout = o.cfg.SandboxIdleTimeout
	}
	configureAgentSandboxLifecycle(ctx, o.provider, &sb, info.ExternalID, idleTimeout)
	logPhase("configure lifecycle", "sandbox_id", sb.ID, "external_id", info.ExternalID, "agent_id", agent.ID, "org_id", orgID, "idle_timeout_ms", idleTimeout.Milliseconds())
	logging.FromContext(ctx).InfoContext(ctx, "agent sandbox created",
		"sandbox_id", sb.ID, "external_id", info.ExternalID, "agent_id", agent.ID)
	logPhase("complete", "sandbox_id", sb.ID, "external_id", info.ExternalID, "agent_id", agent.ID, "org_id", orgID)
	return &sb, nil
}
