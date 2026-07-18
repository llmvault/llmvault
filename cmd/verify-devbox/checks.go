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
	"github.com/golang-jwt/jwt/v5"

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
			}{name: "Docker daemon", command: `test "$(id -u)" = 1000 && test "$(docker info --format '{{.Driver}} {{.DockerRootDir}}')" = "overlay2 /var/lib/docker"`},
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
			return fmt.Errorf("%s exited %d: %s", check.name, result.ExitCode, truncate(result.Result, 4000))
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

func verifyBrowserPreviewCORS(ctx context.Context, url string) error {
	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	const origin = "https://usehivy.com"
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Origin", origin)
	req.Header.Set("User-Agent", "Mozilla/5.0 Hivy-Daytona-Acceptance")
	req.Header.Set("X-Daytona-Skip-Preview-Warning", "true")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(body), 400))
	}
	allowedOrigins := resp.Header.Values("Access-Control-Allow-Origin")
	if len(allowedOrigins) != 1 || allowedOrigins[0] != origin {
		return fmt.Errorf("access-control-allow-origin = %q, want [%q]", allowedOrigins, origin)
	}
	if strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/html") {
		return fmt.Errorf("preview returned HTML instead of Runtime response")
	}
	return nil
}

func verifyBrowserSessionStreamCORS(ctx context.Context, runtimeURL, runtimeSecret string) error {
	const sessionID = "daytona-acceptance-session"
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"aud":        "hivy-runtime",
		"exp":        time.Now().UTC().Add(5 * time.Minute).Unix(),
		"nbf":        time.Now().UTC().Add(-5 * time.Second).Unix(),
		"session_id": sessionID,
		"sandbox_id": "daytona-acceptance-sandbox",
		"scopes":     []string{"stream:read"},
	}).SignedString([]byte(runtimeSecret))
	if err != nil {
		return fmt.Errorf("signing browser stream token: %w", err)
	}

	// Daytona may buffer a new SSE response until Runtime emits its first
	// keepalive frame, so this must exceed the stream's keepalive interval.
	reqCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(
		reqCtx,
		http.MethodGet,
		strings.TrimRight(runtimeURL, "/")+"/sessions/"+sessionID+"/stream?replay=none",
		nil,
	)
	if err != nil {
		return err
	}
	const origin = "https://usehivy.com"
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Origin", origin)
	req.Header.Set("User-Agent", "Mozilla/5.0 Hivy-Daytona-Acceptance")
	req.Header.Set("X-Daytona-Skip-Preview-Warning", "true")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d, want 200", resp.StatusCode)
	}
	if !strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") {
		return fmt.Errorf("content-type = %q, want text/event-stream", resp.Header.Get("Content-Type"))
	}
	allowedOrigins := resp.Header.Values("Access-Control-Allow-Origin")
	if len(allowedOrigins) != 1 || allowedOrigins[0] != origin {
		return fmt.Errorf("access-control-allow-origin = %q, want [%q]", allowedOrigins, origin)
	}
	return nil
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
