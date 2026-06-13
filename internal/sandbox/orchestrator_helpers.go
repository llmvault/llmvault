package sandbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"gorm.io/gorm"
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

func (o *Orchestrator) mergeUserEnvVars(ctx context.Context, envVars map[string]string, encrypted []byte) {
	if o.encKey == nil || len(encrypted) == 0 {
		return
	}
	decrypted, err := o.encKey.DecryptString(encrypted)
	if err != nil {
		logging.Capture(ctx, fmt.Errorf("decrypt user env vars: %w", err))
		return
	}
	var userVars map[string]string
	if err := json.Unmarshal([]byte(decrypted), &userVars); err != nil {
		logging.Capture(ctx, fmt.Errorf("parse user env vars: %w", err))
		return
	}
	for k, v := range userVars {
		envVars[k] = v
	}
}

func (o *Orchestrator) loadOwningAgent(ctx context.Context, agent *model.Agent) (*model.Agent, error) {
	if agent == nil || agent.IsManaged || agent.OrgID == nil {
		return nil, nil
	}
	var owner model.Agent
	err := o.db.WithContext(ctx).
		Where("org_id = ? AND id <> ? AND status <> ?", *agent.OrgID, agent.ID, "archived").
		Order("created_at ASC").
		First(&owner).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &owner, nil
}

func mergeJSONMaps(base model.JSON, override model.JSON) model.JSON {
	if len(base) == 0 && len(override) == 0 {
		return model.JSON{}
	}
	out := make(model.JSON, len(base)+len(override))
	for key, value := range base {
		out[key] = value
	}
	for key, value := range override {
		out[key] = value
	}
	return out
}

func cloneAgentWithInheritedResources(agent *model.Agent, owner *model.Agent) *model.Agent {
	if agent == nil || owner == nil {
		return agent
	}
	clone := *agent
	clone.Resources = mergeJSONMaps(owner.Resources, agent.Resources)
	return &clone
}

func appendUniqueUUIDs(ids []uuid.UUID, seen map[uuid.UUID]bool, values ...uuid.UUID) []uuid.UUID {
	for _, id := range values {
		if seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids
}

func (o *Orchestrator) runSetupCommands(ctx context.Context, sb *model.Sandbox, commands []string) error {
	for _, cmd := range commands {
		if _, err := o.ExecuteCommand(ctx, sb, cmd); err != nil {
			return fmt.Errorf("setup command failed: %s: %w", cmd, err)
		}
	}
	return nil
}
