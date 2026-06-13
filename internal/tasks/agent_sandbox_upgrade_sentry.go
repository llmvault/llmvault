package tasks

import (
	"context"
	"fmt"

	sentrygo "github.com/getsentry/sentry-go"

	"github.com/usehivy/hivy/internal/model"
	sentryobs "github.com/usehivy/hivy/internal/observability/sentry"
)

func annotateAgentSandboxUpgradeSentry(ctx context.Context, upgrade *model.AgentSandboxUpgrade, agent *model.Agent, oldSandbox *model.Sandbox) {
	hub := agentUpgradeHub(ctx)
	if hub == nil || upgrade == nil {
		return
	}
	hub.Scope().SetTag("feature", "agent_sandbox_upgrade")
	hub.Scope().SetTag("agent.upgrade_id", upgrade.ID.String())
	hub.Scope().SetTag("agent.agent_id", upgrade.AgentID.String())
	hub.Scope().SetTag("agent.org_id", upgrade.OrgID.String())
	if oldSandbox != nil {
		hub.Scope().SetTag("agent.old_sandbox_id", oldSandbox.ID.String())
	}
	setAgentUpgradeContext(hub, upgrade, oldSandbox, nil)
	addAgentUpgradeBreadcrumb(ctx, "started", sentrygo.LevelInfo, sentrygo.Context{
		"status": upgrade.Status,
		"phase":  upgrade.Phase,
	})
}

func recordAgentSandboxUpgradePhase(ctx context.Context, upgrade *model.AgentSandboxUpgrade, phase string) {
	hub := agentUpgradeHub(ctx)
	if hub == nil || upgrade == nil {
		return
	}
	hub.Scope().SetTag("agent.upgrade_phase", phase)
	setAgentUpgradeContext(hub, upgrade, nil, nil)
	addAgentUpgradeBreadcrumb(ctx, "phase "+phase, sentrygo.LevelInfo, sentrygo.Context{
		"phase": phase,
	})
}

func recordAgentSandboxUpgradeNewSandbox(ctx context.Context, upgrade *model.AgentSandboxUpgrade, sb *model.Sandbox) {
	hub := agentUpgradeHub(ctx)
	if hub == nil || upgrade == nil || sb == nil {
		return
	}
	hub.Scope().SetTag("agent.new_sandbox_id", sb.ID.String())
	setAgentUpgradeContext(hub, upgrade, nil, sentrygo.Context{
		"new_sandbox_id":          sb.ID.String(),
		"new_sandbox_external_id": sb.ExternalID,
	})
}

func recordAgentSandboxUpgradeBackup(ctx context.Context, upgrade *model.AgentSandboxUpgrade, meta *agentSandboxBackupMetadata) {
	hub := agentUpgradeHub(ctx)
	if hub == nil || upgrade == nil || meta == nil {
		return
	}
	setAgentUpgradeContext(hub, upgrade, nil, sentrygo.Context{
		"backup_key":    meta.Key,
		"backup_sha256": meta.SHA256,
		"backup_bytes":  meta.Bytes,
	})
	addAgentUpgradeBreadcrumb(ctx, "backup captured", sentrygo.LevelInfo, sentrygo.Context{
		"backup_key":   meta.Key,
		"backup_bytes": meta.Bytes,
	})
}

func recordAgentSandboxUpgradeFailure(ctx context.Context, upgrade *model.AgentSandboxUpgrade, phase, message string) {
	hub := agentUpgradeHub(ctx)
	if hub == nil || upgrade == nil {
		return
	}
	hub.Scope().SetTag("agent.upgrade_phase", phase)
	hub.Scope().SetTag("agent.upgrade_status", model.AgentSandboxUpgradeStatusFailed)
	setAgentUpgradeContext(hub, upgrade, nil, sentrygo.Context{
		"failed_phase": phase,
		"error":        message,
	})
	addAgentUpgradeBreadcrumb(ctx, "failed at "+phase, sentrygo.LevelError, sentrygo.Context{
		"phase": phase,
		"error": message,
	})
}

func recordAgentSandboxUpgradeSuccess(ctx context.Context, upgrade *model.AgentSandboxUpgrade) {
	hub := agentUpgradeHub(ctx)
	if hub == nil || upgrade == nil {
		return
	}
	hub.Scope().SetTag("agent.upgrade_phase", model.AgentSandboxUpgradePhaseCompleted)
	hub.Scope().SetTag("agent.upgrade_status", model.AgentSandboxUpgradeStatusSucceeded)
	setAgentUpgradeContext(hub, upgrade, nil, nil)
	addAgentUpgradeBreadcrumb(ctx, "completed", sentrygo.LevelInfo, sentrygo.Context{
		"status": model.AgentSandboxUpgradeStatusSucceeded,
	})
}

func recordAgentSandboxRetire(ctx context.Context, upgrade *model.AgentSandboxUpgrade, sb *model.Sandbox) {
	hub := agentUpgradeHub(ctx)
	if hub == nil || upgrade == nil || sb == nil {
		return
	}
	hub.Scope().SetTag("feature", "agent_sandbox_upgrade")
	hub.Scope().SetTag("agent.upgrade_id", upgrade.ID.String())
	hub.Scope().SetTag("agent.agent_id", upgrade.AgentID.String())
	hub.Scope().SetTag("agent.old_sandbox_id", sb.ID.String())
	setAgentUpgradeContext(hub, upgrade, sb, nil)
	addAgentUpgradeBreadcrumb(ctx, "retire old sandbox", sentrygo.LevelInfo, sentrygo.Context{
		"sandbox_id":  sb.ID.String(),
		"external_id": sb.ExternalID,
	})
}

func agentUpgradeHub(ctx context.Context) *sentrygo.Hub {
	if !sentryobs.Enabled() {
		return nil
	}
	if hub := sentrygo.GetHubFromContext(ctx); hub != nil {
		return hub
	}
	return sentrygo.CurrentHub()
}

func setAgentUpgradeContext(hub *sentrygo.Hub, upgrade *model.AgentSandboxUpgrade, oldSandbox *model.Sandbox, extra sentrygo.Context) {
	data := sentrygo.Context{
		"upgrade_id": upgrade.ID.String(),
		"org_id":     upgrade.OrgID.String(),
		"agent_id":   upgrade.AgentID.String(),
		"status":     upgrade.Status,
		"phase":      upgrade.Phase,
	}
	if upgrade.OldSandboxID != nil {
		data["old_sandbox_id"] = upgrade.OldSandboxID.String()
	}
	if upgrade.NewSandboxID != nil {
		data["new_sandbox_id"] = upgrade.NewSandboxID.String()
	}
	if oldSandbox != nil {
		data["old_sandbox_external_id"] = oldSandbox.ExternalID
		data["old_sandbox_status"] = oldSandbox.Status
	}
	for key, value := range extra {
		data[key] = value
	}
	hub.Scope().SetContext("agent_sandbox_upgrade", data)
}

func addAgentUpgradeBreadcrumb(ctx context.Context, message string, level sentrygo.Level, data sentrygo.Context) {
	hub := agentUpgradeHub(ctx)
	if hub == nil {
		return
	}
	hub.AddBreadcrumb(&sentrygo.Breadcrumb{
		Type:     "default",
		Category: "agent_sandbox_upgrade",
		Message:  fmt.Sprintf("agent sandbox upgrade %s", message),
		Level:    level,
		Data:     data,
	}, nil)
}
