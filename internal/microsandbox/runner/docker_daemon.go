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
HIVY_SANDBOX_DATA_ROOT="${HIVY_SANDBOX_DATA_ROOT:-/workspace/.hivy}"
HIVY_DOCKER_DATA_ROOT="${HIVY_DOCKER_DATA_ROOT:-$HIVY_SANDBOX_DATA_ROOT/docker}"
TMPDIR="${TMPDIR:-$HIVY_SANDBOX_DATA_ROOT/tmp}"
TEMP="${TEMP:-$TMPDIR}"
TMP="${TMP:-$TMPDIR}"
DOCKER_TMPDIR="${DOCKER_TMPDIR:-$TMPDIR/docker}"
export HIVY_SANDBOX_DATA_ROOT HIVY_DOCKER_DATA_ROOT TMPDIR TEMP TMP DOCKER_TMPDIR
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
mkdir -p "$HIVY_DOCKER_DATA_ROOT" "$DOCKER_TMPDIR" "$HIVY_SANDBOX_DATA_ROOT/logs" /var/run
chmod 1777 "$TMPDIR" "$DOCKER_TMPDIR" 2>/dev/null || true
if command -v mount >/dev/null 2>&1 && [ -d /tmp ]; then
	if ! mount | grep -F "$TMPDIR on /tmp " >/dev/null 2>&1; then
		mount --bind "$TMPDIR" /tmp 2>/dev/null || true
	fi
fi
if [ -d /var/lib/docker ] && [ ! -L /var/lib/docker ]; then
	if ! command -v mountpoint >/dev/null 2>&1 || ! mountpoint -q /var/lib/docker; then
		rmdir /var/lib/docker 2>/dev/null || true
	fi
fi
if [ ! -e /var/lib/docker ]; then
	ln -s "$HIVY_DOCKER_DATA_ROOT" /var/lib/docker
fi
dockerd --host=unix:///var/run/docker.sock --data-root="$HIVY_DOCKER_DATA_ROOT" >"$HIVY_SANDBOX_DATA_ROOT/logs/dockerd.log" 2>&1 &
for _ in $(seq 1 60); do
	if docker info >/dev/null 2>&1; then
		echo started
		exit 0
	fi
	sleep 1
done
cat "$HIVY_SANDBOX_DATA_ROOT/logs/dockerd.log" >&2 || true
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
