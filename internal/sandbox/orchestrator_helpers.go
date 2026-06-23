package sandbox

import (
	"context"
	"fmt"
	"time"

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

func configureAgentSandboxLifecycle(ctx context.Context, provider Provider, sb *model.Sandbox, externalID string, idleTimeout time.Duration) {
	if provider.ID() == ProviderMicrosandbox {
		minutes := 5
		if idleTimeout > 0 {
			minutes = int((idleTimeout + time.Minute - 1) / time.Minute)
			if minutes < 1 {
				minutes = 1
			}
		}
		if err := provider.SetAutoStop(ctx, externalID, minutes); err != nil {
			logging.Capture(ctx, fmt.Errorf("set microsandbox auto-stop sandbox %s: %w", sb.ID, err))
		}
		return
	}
	disableProviderLifecycle(ctx, provider, sb, externalID)
}

func (o *Orchestrator) providerID() string {
	if o == nil || o.provider == nil {
		return ""
	}
	return o.provider.ID()
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
