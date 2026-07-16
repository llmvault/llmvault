package runner

import (
	"context"
	"fmt"
	"time"

	microsandbox "github.com/superradcompany/microsandbox/sdk/go"

	"github.com/usehivy/hivy/internal/logging"
)

func (m *MicrosandboxBackend) startSandboxLocked(ctx context.Context, sandboxID string) error {
	actual := m.actualState(ctx, sandboxID)
	if actual.infrastructureRunning() {
		m.setSandboxStatus(sandboxID, "running")
		return nil
	}
	if actual.hasHostResidue() {
		if err := m.stopStaleSameSandbox(ctx, sandboxID); err != nil {
			return err
		}
	}

	if err := m.startSandboxNative(ctx, sandboxID); err != nil {
		if !isAttachedDiskError(err) {
			return err
		}
		actual = m.actualState(ctx, sandboxID)
		if actual.infrastructureRunning() {
			m.setSandboxStatus(sandboxID, "running")
			return nil
		}
		if actual.hasHostResidue() {
			if stopErr := m.stopStaleSameSandbox(ctx, sandboxID); stopErr != nil {
				return fmt.Errorf("start failed with attached disk and stale stop failed: %v: %w", err, stopErr)
			}
			if retryErr := m.startSandboxNative(ctx, sandboxID); retryErr != nil {
				return fmt.Errorf("retry start after stale stop: %w", retryErr)
			}
		} else {
			return err
		}
	}

	if err := m.waitForInfrastructureRunning(ctx, sandboxID); err != nil {
		return fmt.Errorf("verify running: %w", err)
	}
	m.setSandboxStatus(sandboxID, "running")
	return nil
}

func (m *MicrosandboxBackend) startSandboxNative(ctx context.Context, sandboxID string) error {
	sb, err := microsandbox.StartSandboxDetached(ctx, sandboxID)
	if err != nil {
		return err
	}
	if err := sb.Detach(ctx); err != nil {
		return err
	}
	return nil
}

func (m *MicrosandboxBackend) stopSandboxLocked(ctx context.Context, sandboxID string) error {
	actual := m.actualState(ctx, sandboxID)
	if actual.runtimeStopped() {
		m.logCleanupPending(ctx, sandboxID, actual)
		m.setSandboxStatus(sandboxID, "stopped")
		return nil
	}
	if err := m.stopSandboxNative(ctx, sandboxID); err != nil {
		if stopErr := ignoreAlreadyStopped(err); stopErr != nil {
			return stopErr
		}
	}
	if err := m.waitForRuntimeStopped(ctx, sandboxID); err != nil {
		if forceErr := m.forceStopActual(ctx, sandboxID); forceErr != nil {
			return fmt.Errorf("verify stopped: %v; force stop: %w", err, forceErr)
		}
		if retryErr := m.waitForRuntimeStopped(ctx, sandboxID); retryErr != nil {
			return fmt.Errorf("verify stopped after force stop: %w", retryErr)
		}
	}
	m.logCleanupPending(ctx, sandboxID, m.actualState(ctx, sandboxID))
	m.setSandboxStatus(sandboxID, "stopped")
	return nil
}

func (m *MicrosandboxBackend) logCleanupPending(ctx context.Context, sandboxID string, actual actualSandboxState) {
	if actual.VolumeDiskFDs == 0 {
		return
	}
	logging.FromContext(ctx).WarnContext(ctx, "sandbox runtime stopped with volume cleanup pending",
		"sandbox_id", sandboxID, "volume_disk_fds", actual.VolumeDiskFDs)
}

func (m *MicrosandboxBackend) stopSandboxNative(ctx context.Context, sandboxID string) error {
	handle, err := microsandbox.GetSandbox(ctx, sandboxID)
	if err != nil {
		return ignoreNotFound(err)
	}
	return handle.Stop(ctx, microsandbox.WithStopTimeout(15*time.Second))
}

func (m *MicrosandboxBackend) stopStaleSameSandbox(ctx context.Context, sandboxID string) error {
	_ = m.stopSandboxNative(ctx, sandboxID)
	if err := m.forceStopActual(ctx, sandboxID); err != nil {
		return err
	}
	if err := m.waitForFullyStopped(ctx, sandboxID); err != nil {
		return fmt.Errorf("waiting for stale same-sandbox VM to stop: %w", err)
	}
	return nil
}

func (m *MicrosandboxBackend) deleteSandboxLocked(ctx context.Context, sandboxID string) error {
	handle, err := microsandbox.GetSandbox(ctx, sandboxID)
	if err == nil {
		_ = handle.Stop(ctx, microsandbox.WithStopTimeout(15*time.Second))
		_ = handle.Remove(ctx)
	}
	_ = m.forceStopActual(ctx, sandboxID)
	_ = m.waitForFullyStopped(ctx, sandboxID)
	_ = microsandbox.RemoveVolume(ctx, workspaceVolumeName(sandboxID))
	_ = microsandbox.RemoveVolume(ctx, dockerDataVolumeName(sandboxID))
	m.mu.Lock()
	released := mapValues(m.ports[sandboxID])
	delete(m.ports, sandboxID)
	delete(m.sandboxes, sandboxID)
	m.mu.Unlock()
	m.allocator.release(released)
	return nil
}

func (m *MicrosandboxBackend) sandboxPorts(sandboxID string) map[int]int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return cloneIntMap(m.ports[sandboxID])
}

func (m *MicrosandboxBackend) hostPortForGuest(sandboxID string, guestPort int) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.ports[sandboxID][guestPort]
}
