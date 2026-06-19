package runner

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/usehivy/hivy/internal/logging"
)

const dockerDaemonBootstrapTimeoutSeconds = 90

const dockerDaemonBootstrapCommand = `
set -eu
if ! command -v dockerd >/dev/null 2>&1; then
	echo no-dockerd
	exit 0
fi
if ! command -v docker >/dev/null 2>&1; then
	echo no-docker
	exit 0
fi
if docker info >/dev/null 2>&1; then
	echo already-running
	exit 0
fi
for pid in $(pidof dockerd containerd containerd-shim-runc-v2 2>/dev/null || true); do
	kill "$pid" >/dev/null 2>&1 || true
done
sleep 1
for pid in $(pidof dockerd containerd containerd-shim-runc-v2 2>/dev/null || true); do
	kill -9 "$pid" >/dev/null 2>&1 || true
done
rm -f /var/run/docker.sock /run/docker.pid /var/run/docker.pid
rm -f /run/containerd/containerd.sock /var/run/containerd/containerd.sock
rm -rf /run/docker /var/run/docker /run/containerd /var/run/containerd
mkdir -p /var/lib/docker /var/run
dockerd --host=unix:///var/run/docker.sock --data-root=/var/lib/docker >/tmp/dockerd.log 2>&1 &
for _ in $(seq 1 60); do
	if docker info >/dev/null 2>&1; then
		echo started
		exit 0
	fi
	sleep 1
done
cat /tmp/dockerd.log >&2 || true
exit 1
`

func (m *MicrosandboxBackend) ensureDockerDaemon(ctx context.Context, sandboxID string) error {
	start := time.Now()
	resp, err := m.Exec(ctx, sandboxID, dockerDaemonBootstrapCommand, dockerDaemonBootstrapTimeoutSeconds)
	if err != nil {
		return fmt.Errorf("bootstrap docker daemon: %w", err)
	}
	if resp.ExitCode != 0 {
		return fmt.Errorf("bootstrap docker daemon exited with code %d: %s", resp.ExitCode, strings.TrimSpace(resp.Stdout+resp.Stderr))
	}
	logging.FromContext(ctx).InfoContext(ctx, "sandbox docker daemon bootstrap completed",
		"sandbox_id", sandboxID,
		"duration_ms", time.Since(start).Milliseconds(),
		"status", strings.TrimSpace(resp.Stdout),
	)
	return nil
}
