package runner

import (
	"context"
	"fmt"
	"time"
)

func (m *MicrosandboxBackend) StartSandbox(ctx context.Context, sandboxID string) error {
	unlock := m.lifecycle.Lock(sandboxID)
	defer unlock()
	return m.startSandboxLocked(ctx, sandboxID)
}

func (m *MicrosandboxBackend) StopSandbox(ctx context.Context, sandboxID string) error {
	unlock := m.lifecycle.Lock(sandboxID)
	defer unlock()
	return m.stopSandboxLocked(ctx, sandboxID)
}

func (m *MicrosandboxBackend) SandboxLabels(sandboxID string) (map[string]string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	state, ok := m.sandboxes[sandboxID]
	if !ok {
		return nil, false
	}
	return cloneStringMap(state.Labels), true
}

func (m *MicrosandboxBackend) EnsureReady(ctx context.Context, sandboxID string, req EnsureReadyRequest) (*EnsureReadyResponse, error) {
	unlock := m.lifecycle.Lock(sandboxID)
	defer unlock()
	if err := m.startSandboxLocked(ctx, sandboxID); err != nil {
		return nil, err
	}
	hostPort := m.hostPortForGuest(sandboxID, req.GuestPort)
	if hostPort == 0 {
		return nil, fmt.Errorf("guest port %d is not published for sandbox %s", req.GuestPort, sandboxID)
	}
	timeout := startVerifyTimeout
	if req.TimeoutSeconds > 0 {
		timeout = time.Duration(req.TimeoutSeconds) * time.Second
	}
	readyCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := waitForCondition(readyCtx, timeout, func() (bool, string) {
		return tcpPortOpen(hostPort, actualProbeTimeout), fmt.Sprintf("host_port=%d port_open=false", hostPort)
	}); err != nil {
		return nil, err
	}
	if req.HealthCheck != nil {
		if err := waitForHTTPHealthCheck(readyCtx, sandboxID, req.GuestPort, hostPort, *req.HealthCheck); err != nil {
			return nil, err
		}
	}
	m.setSandboxStatus(sandboxID, "running")
	return &EnsureReadyResponse{Status: "running", HostPort: hostPort}, nil
}

func (m *MicrosandboxBackend) DeleteSandbox(ctx context.Context, sandboxID string) error {
	unlock := m.lifecycle.Lock(sandboxID)
	defer unlock()
	return m.deleteSandboxLocked(ctx, sandboxID)
}
