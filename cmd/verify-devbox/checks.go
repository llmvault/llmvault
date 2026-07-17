package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	daytona "github.com/daytona/clients/sdk-go/pkg/daytona"
	"github.com/daytona/clients/sdk-go/pkg/types"

	"github.com/usehivy/hivy/internal/agentruntime"
)

func sandboxParams(snapshot, runtimeSecret string) types.SnapshotParams {
	return types.SnapshotParams{
		SandboxBaseParams: types.SandboxBaseParams{
			Name:   "hivy-daytona-acceptance-" + time.Now().UTC().Format("20060102-150405"),
			User:   daytonaUser,
			Public: false,
			EnvVars: map[string]string{
				"HOME":                   daytonaHome,
				"HIVY_SANDBOX_DATA_ROOT": daytonaHome + "/.hivy",
				"HIVY_DB_PATH":           daytonaDBPath,
				"HIVY_WORKSPACE_ROOT":    daytonaWorkspaceRoot,
				runtimeSecretEnv:         runtimeSecret,
			},
			NetworkBlockAll: false,
		},
		Snapshot: snapshot,
	}
}

func verifySandboxProcess(ctx context.Context, sandbox *daytona.Sandbox, developer bool) error {
	runtimeProcessCommand := `pgrep -f '^/usr/local/bin/hivy-sandboxes-runtime$' >/dev/null`
	if developer {
		runtimeProcessCommand = `for _ in $(seq 1 45); do pgrep -f '^/usr/local/bin/hivy-sandboxes-runtime$' >/dev/null && exit 0; sleep 1; done; cat "$HIVY_SANDBOX_DATA_ROOT/logs/dockerd.log" 2>/dev/null || true; exit 1`
	}
	checks := []struct {
		name    string
		command string
	}{
		{name: "non-root Daytona user", command: `test "$(id -u)" = 1000 && test "$(whoami)" = daytona`},
		{name: "Daytona home and workspace", command: `test "$HOME" = /home/daytona && test -w /home/daytona && test "$(readlink -f /workspace)" = /home/daytona`},
		{name: "runtime process", command: runtimeProcessCommand},
		{name: "no systemd dependency", command: `test "$(cat /proc/1/comm)" != systemd`},
		{name: "outbound network", command: `curl -fsS --max-time 20 https://api.github.com/zen >/dev/null`},
	}
	if developer {
		checks = append(checks,
			struct {
				name    string
				command string
			}{name: "Docker daemon", command: `test "$(id -u)" = 1000 && docker info >/dev/null`},
			struct {
				name    string
				command string
			}{name: "Docker Compose", command: `set -eu; d=$(mktemp -d); trap 'docker compose -f "$d/compose.yaml" down --remove-orphans >/dev/null 2>&1 || true; rm -rf "$d"' EXIT; printf '%s\n' 'services:' '  smoke:' '    image: alpine:3.22' '    command: ["sh", "-c", "echo compose-ok; sleep 30"]' >"$d/compose.yaml"; docker compose -f "$d/compose.yaml" up -d --wait; docker compose -f "$d/compose.yaml" logs smoke | grep -q compose-ok`},
		)
	} else {
		checks = append(checks, struct {
			name    string
			command string
		}{name: "default image excludes Docker", command: `! command -v docker >/dev/null 2>&1`})
	}

	for _, check := range checks {
		log.Printf("Checking %s...", check.name)
		result, err := sandbox.Process.ExecuteCommand(ctx, check.command)
		if err != nil {
			return fmt.Errorf("%s: %w", check.name, err)
		}
		if result.ExitCode != 0 {
			return fmt.Errorf("%s exited %d: %s", check.name, result.ExitCode, truncate(result.Result, 500))
		}
	}
	return nil
}

func waitForRuntimeHealth(ctx context.Context, client *agentruntime.Client) error {
	deadline := time.Now().Add(2 * time.Minute)
	var lastErr error
	for time.Now().Before(deadline) {
		if err := client.Healthz(ctx); err == nil {
			log.Printf("Runtime health check passed.")
			return nil
		} else {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return fmt.Errorf("waiting for Runtime health: %w", lastErr)
}

func httpGet(ctx context.Context, url, token string) (int, string, error) {
	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return 0, "", err
	}
	if token != "" {
		req.Header.Set("x-daytona-preview-token", token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return resp.StatusCode, "", err
	}
	return resp.StatusCode, string(body), nil
}

func truncate(text string, limit int) string {
	text = strings.ReplaceAll(text, "\n", " ")
	if len(text) <= limit {
		return text
	}
	return text[:limit] + "…"
}
