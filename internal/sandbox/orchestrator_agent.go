package sandbox

import (
	"context"
	"time"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/model"
)

const (
	AgentSandboxPort        = 7080
	agentHealthTimeout      = 20 * time.Second
	agentHealthInterval     = 200 * time.Millisecond
	agentHealthProbeTimeout = 750 * time.Millisecond
)

func (o *Orchestrator) CreateAgentSandbox(ctx context.Context, agent *model.Agent, secrets *agentruntime.StartupSecrets) (*model.Sandbox, error) {
	return o.CreateAgentSandboxWithRuntimeOptions(ctx, agent, secrets, agentruntime.RuntimeConfigOptions{})
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
