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

func (o *Orchestrator) SupportsInPlaceUpgrade(sb *model.Sandbox) bool {
	if o == nil || o.provider == nil {
		return false
	}
	if err := o.ensureSandboxProvider(sb); err != nil {
		return false
	}
	_, ok := o.provider.(UpgradeableProvider)
	return ok
}

func (o *Orchestrator) UpgradeAgentSandboxInPlace(ctx context.Context, agent *model.Agent, sb *model.Sandbox, secrets *agentruntime.StartupSecrets) (*model.Sandbox, error) {
	if agent == nil || agent.OrgID == nil {
		return nil, fmt.Errorf("UpgradeAgentSandboxInPlace: agent must have org_id")
	}
	if sb == nil {
		return nil, fmt.Errorf("UpgradeAgentSandboxInPlace: sandbox is required")
	}
	if secrets == nil || secrets.ProxyToken == "" || secrets.ProxyTokenJTI == "" {
		return nil, fmt.Errorf("UpgradeAgentSandboxInPlace: proxy token is required")
	}
	if err := o.ensureSandboxProvider(sb); err != nil {
		return nil, err
	}
	upgrader, ok := o.provider.(UpgradeableProvider)
	if !ok {
		return nil, fmt.Errorf("provider %q does not support in-place upgrade: %w", o.provider.ID(), ErrUnsupported)
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
	runtimeSecret, err := o.encKey.DecryptString(sb.EncryptedRuntimeSecret)
	if err != nil {
		return nil, fmt.Errorf("decrypt runtime secret: %w", err)
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
	snapshotID := RuntimeTemplateRefForImageRef(o.cfg, runtimeImage, sandboxSize)
	if templateRef != "" {
		snapshotID = templateRef
	}

	bugsinkDashboardURL := agentruntime.BugsinkDashboardBaseURL(ctx, o.db, orgID, *agent)
	glitchTipDashboardURL := agentruntime.GlitchTipDashboardBaseURL(ctx, o.db, orgID, *agent)
	envVars := agentSandboxEnvVars(o.cfg, runtimeSecret, sb, orgID, agent, secrets, gitIdentity, bugsinkDashboardURL, glitchTipDashboardURL)
	labels := map[string]string{
		"org_id":        orgID.String(),
		"sandbox_id":    sb.ID.String(),
		"agent_id":      agent.ID.String(),
		"harness":       "agent-sandbox",
		"sandbox_size":  sandboxSize,
		"sandbox_image": sandboxImage,
	}

	info, err := upgrader.UpgradeSandbox(ctx, sb.ExternalID, UpgradeSandboxOpts{
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
		return nil, fmt.Errorf("provider upgrade: %w", err)
	}

	sandboxURL, err := o.provider.GetEndpoint(ctx, info.ExternalID, AgentSandboxPort)
	if err != nil {
		return nil, fmt.Errorf("getting agent runtime endpoint: %w", err)
	}
	now := time.Now()
	expiresAt := now.Add(runtimeURLTTL)
	if err := o.db.WithContext(ctx).Model(sb).Updates(map[string]any{
		"sandbox_template_id":    agent.SandboxTemplateID,
		"snapshot_id":            snapshotID,
		"runtime_url":            sandboxURL,
		"runtime_url_expires_at": expiresAt,
		"status":                 string(StatusRunning),
		"last_active_at":         now,
		"exposed_ports":          model.SandboxExposedPortsInt64Array(exposedPorts),
		"error_message":          nil,
	}).Error; err != nil {
		return nil, fmt.Errorf("updating upgraded sandbox: %w", err)
	}
	sb.SandboxTemplateID = agent.SandboxTemplateID
	sb.SnapshotID = &snapshotID
	sb.RuntimeURL = sandboxURL
	sb.RuntimeURLExpiresAt = &expiresAt
	sb.Status = string(StatusRunning)
	sb.LastActiveAt = &now
	sb.ExposedPorts = model.SandboxExposedPortsInt64Array(exposedPorts)
	sb.ErrorMessage = nil

	idleTimeout := time.Duration(0)
	if o.cfg != nil {
		idleTimeout = o.cfg.SandboxIdleTimeout
	}
	configureAgentSandboxLifecycle(ctx, o.provider, sb, info.ExternalID, idleTimeout)
	logging.FromContext(ctx).InfoContext(ctx, "agent sandbox upgraded in place",
		"sandbox_id", sb.ID, "external_id", info.ExternalID, "agent_id", agent.ID)
	return sb, nil
}
