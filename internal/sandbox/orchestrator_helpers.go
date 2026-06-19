package sandbox

import (
	"context"
	"fmt"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

func disableProviderLifecycle(ctx context.Context, provider Provider, sb *model.Sandbox, externalID string) {
	if err := provider.SetAutoStop(ctx, externalID, 0); err != nil {
		logging.Capture(ctx, fmt.Errorf("disable provider auto-stop sandbox %s: %w", sb.ID, err))
	}
	if err := provider.SetAutoArchive(ctx, externalID, 0); err != nil {
		logging.Capture(ctx, fmt.Errorf("disable provider auto-archive sandbox %s: %w", sb.ID, err))
	}
}

func (o *Orchestrator) providerID() string {
	if o == nil || o.provider == nil {
		return ""
	}
	return o.provider.ID()
}

func (o *Orchestrator) runtimeLayout() RuntimeLayout {
	layout := RuntimeLayout{
		AgentRepoDir:     "/work/repos",
		WorkspaceRepoDir: "/workspace/repos",
	}
	if o != nil && o.provider != nil {
		providerLayout := o.provider.RuntimeLayout()
		if providerLayout.AgentRepoDir != "" {
			layout.AgentRepoDir = providerLayout.AgentRepoDir
		}
		if providerLayout.WorkspaceRepoDir != "" {
			layout.WorkspaceRepoDir = providerLayout.WorkspaceRepoDir
		}
	}
	return layout
}

func (o *Orchestrator) ensureSandboxProvider(sb *model.Sandbox) error {
	if sb == nil {
		return fmt.Errorf("sandbox is nil")
	}
	expected := o.providerID()
	if expected == "" {
		return fmt.Errorf("sandbox provider not configured")
	}
	if sb.ProviderID == "" {
		sb.ProviderID = ProviderDaytona
	}
	if sb.ProviderID != expected {
		return fmt.Errorf("sandbox %s was created by provider %q; active provider is %q", sb.ID, sb.ProviderID, expected)
	}
	return nil
}

func (o *Orchestrator) ensureTemplateProvider(tmpl *model.SandboxTemplate) error {
	if tmpl == nil {
		return fmt.Errorf("sandbox template is nil")
	}
	expected := o.providerID()
	if expected == "" {
		return fmt.Errorf("sandbox provider not configured")
	}
	if tmpl.ProviderID == "" {
		tmpl.ProviderID = ProviderDaytona
	}
	if tmpl.ProviderID != expected {
		return fmt.Errorf("sandbox template %s was created by provider %q; active provider is %q", tmpl.ID, tmpl.ProviderID, expected)
	}
	return nil
}
