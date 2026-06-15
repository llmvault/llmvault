package runner

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

const agentRuntimeBootstrapTimeoutSeconds = 60

const agentRuntimeBootstrapCommand = `
set -eu
if [ -z "${HIVY_RUNTIME_SECRET:-}" ]; then
	echo no-runtime-secret
	exit 0
fi
if [ ! -x /usr/local/bin/hivy-sandboxes-runtime ]; then
	echo no-runtime-binary
	exit 0
fi
bind="${HIVY_RUNTIME_BIND_ADDR:-0.0.0.0:7080}"
port="${bind##*:}"
if pgrep -f '/usr/local/bin/hivy-sandboxes-runtime' >/dev/null 2>&1; then
	if curl -fsS -H "Authorization: Bearer ${HIVY_RUNTIME_SECRET}" "http://127.0.0.1:${port}/healthz" >/dev/null; then
		echo already-running
		exit 0
	fi
fi
if [ -x /usr/local/bin/hivy-runtime-entrypoint ]; then
	HIVY_RUNTIME_START_DOCKERD=0 nohup /usr/local/bin/hivy-runtime-entrypoint /usr/local/bin/hivy-sandboxes-runtime >/tmp/hivy-runtime.log 2>&1 &
else
	nohup /usr/local/bin/hivy-sandboxes-runtime >/tmp/hivy-runtime.log 2>&1 &
fi
for _ in $(seq 1 45); do
	if curl -fsS -H "Authorization: Bearer ${HIVY_RUNTIME_SECRET}" "http://127.0.0.1:${port}/healthz" >/dev/null; then
		echo started
		exit 0
	fi
	sleep 1
done
cat /tmp/hivy-runtime.log >&2 || true
exit 1
`

func (m *MicrosandboxBackend) ensureAgentRuntime(ctx context.Context, sandboxID string) error {
	start := time.Now()
	resp, err := m.Exec(ctx, sandboxID, agentRuntimeBootstrapCommand, agentRuntimeBootstrapTimeoutSeconds)
	if err != nil {
		return fmt.Errorf("bootstrap agent runtime: %w", err)
	}
	if resp.ExitCode != 0 {
		return fmt.Errorf("bootstrap agent runtime exited with code %d: %s", resp.ExitCode, strings.TrimSpace(resp.Stdout+resp.Stderr))
	}
	status := strings.TrimSpace(resp.Stdout)
	slog.Info("sandbox agent runtime bootstrap completed",
		"sandbox_id", sandboxID,
		"duration_ms", time.Since(start).Milliseconds(),
		"status", status,
	)
	return nil
}
