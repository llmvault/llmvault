package runner

import (
	"context"

	microsandbox "github.com/superradcompany/microsandbox/sdk/go"
)

func (m *MicrosandboxBackend) Status(ctx context.Context) (map[string]any, error) {
	handles, err := microsandbox.ListSandboxes(ctx)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	knownSandboxes := len(m.sandboxes)
	publishedPorts := 0
	for _, ports := range m.ports {
		publishedPorts += len(ports)
	}
	m.mu.Unlock()
	return map[string]any{
		"running_sandboxes": len(handles),
		"known_sandboxes":   knownSandboxes,
		"published_ports":   publishedPorts,
	}, nil
}
