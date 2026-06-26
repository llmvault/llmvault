package sandbox

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

const (
	AgentSandboxPort    = 7080
	agentHealthTimeout  = 4 * time.Minute
	agentHealthInterval = 2 * time.Second
)

func (o *Orchestrator) CreateAgentSandbox(ctx context.Context, agent *model.Agent, secrets *agentruntime.StartupSecrets) (*model.Sandbox, error) {
	return o.CreateAgentSandboxWithRuntimeOptions(ctx, agent, secrets, agentruntime.RuntimeConfigOptions{})
}

func (o *Orchestrator) CreateAgentSandboxWithRuntimeOptions(ctx context.Context, agent *model.Agent, secrets *agentruntime.StartupSecrets, runtimeOptions agentruntime.RuntimeConfigOptions) (*model.Sandbox, error) {
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
	exposedPorts, err := o.loadOrgSandboxExposedPorts(ctx, orgID)
	if err != nil {
		return nil, err
	}

	gitIdentity, err := o.loadAgentGitIdentity(ctx, agent)
	if err != nil {
		return nil, fmt.Errorf("loading agent git identity: %w", err)
	}

	runtimeSecret, err := generateRandomHex(32)
	if err != nil {
		return nil, fmt.Errorf("generating runtime secret: %w", err)
	}
	encryptedSecret, err := o.encKey.EncryptString(runtimeSecret)
	if err != nil {
		return nil, fmt.Errorf("encrypting runtime secret: %w", err)
	}

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
	sandboxImage := model.NormalizeSandboxImage(agent.SandboxImage)
	runtimeImage := AgentRuntimeImageRef(o.cfg, sandboxImage)
	warmProfile := AgentWarmPoolProfile(o.cfg, sandboxImage, sandboxSize)
	snapshotID := RuntimeTemplateRefForImageRef(o.cfg, runtimeImage, sandboxSize)
	if templateRef != "" {
		snapshotID = templateRef
	}
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

	bugsinkDashboardURL := agentruntime.BugsinkDashboardBaseURL(ctx, o.db, orgID, *agent)
	glitchTipDashboardURL := agentruntime.GlitchTipDashboardBaseURL(ctx, o.db, orgID, *agent)
	envVars := agentSandboxEnvVars(o.cfg, runtimeSecret, &sb, orgID, agent, secrets, gitIdentity, bugsinkDashboardURL, glitchTipDashboardURL)
	labels := map[string]string{
		"org_id":        orgID.String(),
		"sandbox_id":    sb.ID.String(),
		"agent_id":      agent.ID.String(),
		"harness":       "agent-sandbox",
		"sandbox_size":  sandboxSize,
		"sandbox_image": sandboxImage,
	}

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
			return &sb, nil
		}
		logging.FromContext(ctx).InfoContext(ctx, "agent sandbox warm pool empty; creating directly",
			"sandbox_id", sb.ID, "agent_id", agent.ID, "sandbox_image", sandboxImage, "sandbox_size", sandboxSize)
	}

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

	sandboxURL, err := o.provider.GetEndpoint(ctx, info.ExternalID, AgentSandboxPort)
	if err != nil {
		o.cleanupFailedSandbox(ctx, &sb, info.ExternalID, "get endpoint failed")
		return nil, fmt.Errorf("getting agent runtime endpoint: %w", err)
	}

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

	if err := o.waitForAgentRuntimeLive(ctx, &sb); err != nil {
		o.cleanupFailedSandbox(ctx, &sb, info.ExternalID, "agent runtime not live")
		return nil, fmt.Errorf("waiting for agent runtime: %w", err)
	}
	configPush.ProxyToken = startupProxyToken
	if err := o.pushAgentRuntimeConfig(ctx, &sb, "create", configPush); err != nil {
		o.cleanupFailedSandbox(ctx, &sb, info.ExternalID, "agent runtime config push failed")
		return nil, err
	}

	idleTimeout := time.Duration(0)
	if o.cfg != nil {
		idleTimeout = o.cfg.SandboxIdleTimeout
	}
	configureAgentSandboxLifecycle(ctx, o.provider, &sb, info.ExternalID, idleTimeout)
	logging.FromContext(ctx).InfoContext(ctx, "agent sandbox created",
		"sandbox_id", sb.ID, "external_id", info.ExternalID, "agent_id", agent.ID)
	return &sb, nil
}

func (o *Orchestrator) shouldTryAgentWarmPool(templateRef string, exposedPorts []int) bool {
	if templateRef != "" {
		return false
	}
	capable, ok := o.provider.(WarmPoolCapable)
	if !ok || !capable.UsesWarmPool() {
		return false
	}
	if o.provider.ID() == ProviderMicrosandbox && !warmPoolSupportsDefaultPreviewPorts(exposedPorts) {
		return false
	}
	return true
}

func warmPoolSupportsDefaultPreviewPorts(exposedPorts []int) bool {
	allowed := map[int]struct{}{AgentSandboxPort: {}}
	for _, port := range model.DefaultSandboxExposedPorts() {
		allowed[port] = struct{}{}
	}
	for _, port := range exposedPorts {
		if _, ok := allowed[port]; !ok {
			return false
		}
	}
	return true
}
