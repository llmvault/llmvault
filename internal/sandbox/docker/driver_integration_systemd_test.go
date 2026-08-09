package docker

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sandbox"
)

// systemdIntegrationImageRef is the runtime image used by the systemd-mode
// integration test. Unlike the other tests it cannot use a throwaway alpine
// image (systemd mode requires the real image's systemd + units + env
// generator), so the test skips when the image has not been built locally.
const (
	systemdIntegrationImageRef       = "ghcr.io/usehivy/hivy-sandboxes-runtime:runtime"
	systemdDockerIntegrationImageRef = "ghcr.io/usehivy/hivy-sandboxes-runtime-developers:runtime"
)

func TestDockerDriverSystemdModeBootsSystemdAndRuntimeService(t *testing.T) {
	ctx := context.Background()
	driver := newIntegrationDriverWithConfig(t, ctx, Config{
		RuntimeOrigin:        "http://127.0.0.1",
		ContainerLabelPrefix: integrationLabelPrefix,
		Systemd:              true,
	})

	imageRef := systemdIntegrationImageRef
	if override := strings.TrimSpace(os.Getenv("HIVY_SANDBOX_DOCKER_TEST_SYSTEMD_IMAGE")); override != "" {
		imageRef = override
	}
	if _, err := driver.cli.ImageInspect(ctx, imageRef); err != nil {
		t.Skipf("systemd integration image %s not available locally (build with `make sandbox-runtime-image`): %v", imageRef, err)
	}

	info, err := driver.CreateSandbox(ctx, sandbox.CreateSandboxOpts{
		Name:        "hivy-docker-systemd-integration",
		TemplateRef: imageRef,
		EnvVars: map[string]string{
			// The runtime refuses to start without its bearer token; the
			// orchestrator always provides it.
			"HIVY_RUNTIME_SECRET": "docker-systemd-integration-secret",
			"HIVY_DOCKER_TEST":    "ok",
		},
		Labels: map[string]string{"test": "docker-driver-systemd"},
	})
	if err != nil {
		t.Fatalf("CreateSandbox: %v", err)
	}
	t.Cleanup(func() {
		_ = driver.DeleteSandbox(context.WithoutCancel(ctx), info.ExternalID)
	})

	output, err := driver.ExecuteCommand(ctx, info.ExternalID, "ps -p 1 -o comm=")
	if err != nil {
		t.Fatalf("ExecuteCommand pid1: %v output=%q", err, output)
	}
	if got := strings.TrimSpace(output); got != "systemd" {
		t.Fatalf("PID 1 comm = %q, want systemd", got)
	}

	waitForCommandOutput(t, ctx, driver, info.ExternalID, "systemctl is-active hivy-runtime", "active")

	url, err := driver.GetEndpoint(ctx, info.ExternalID, sandbox.AgentSandboxPort)
	if err != nil {
		t.Fatalf("GetEndpoint: %v", err)
	}
	assertHTTPStatusOK(t, ctx, url+"/healthz")

	// The env generator must forward the container env into the unit: the
	// sandbox env vars have to be visible inside hivy-runtime.service.
	output, err = driver.ExecuteCommand(ctx, info.ExternalID,
		`tr '\0' '\n' </proc/"$(systemctl show hivy-runtime -p MainPID --value)"/environ | grep -x 'HIVY_DOCKER_TEST=ok'`)
	if err != nil {
		t.Fatalf("ExecuteCommand service environ: %v output=%q", err, output)
	}

	stopStart := time.Now()
	if err := driver.StopSandbox(ctx, info.ExternalID); err != nil {
		t.Fatalf("StopSandbox: %v", err)
	}
	if elapsed := time.Since(stopStart); elapsed > 12*time.Second {
		t.Fatalf("StopSandbox took %s; systemd shutdown should beat the stop timeout", elapsed)
	}
	status, err := driver.GetStatus(ctx, info.ExternalID)
	if err != nil {
		t.Fatalf("GetStatus stopped: %v", err)
	}
	if status != sandbox.StatusStopped {
		t.Fatalf("status after stop = %s, want %s", status, sandbox.StatusStopped)
	}
}

func TestDockerDriverSystemdModeStartsNestedDocker(t *testing.T) {
	ctx := context.Background()
	driver := newIntegrationDriverWithConfig(t, ctx, Config{
		RuntimeOrigin:        "http://127.0.0.1",
		ContainerLabelPrefix: integrationLabelPrefix,
		Systemd:              true,
	})

	imageRef := systemdDockerIntegrationImageRef
	if override := strings.TrimSpace(os.Getenv("HIVY_SANDBOX_DOCKER_TEST_DEVELOPER_IMAGE")); override != "" {
		imageRef = override
	}
	if _, err := driver.cli.ImageInspect(ctx, imageRef); err != nil {
		t.Skipf("developer systemd integration image %s is not available locally: %v", imageRef, err)
	}

	info, err := driver.CreateSandbox(ctx, sandbox.CreateSandboxOpts{
		Name:        "hivy-docker-dind-integration",
		TemplateRef: imageRef,
		EnvVars: map[string]string{
			"HIVY_RUNTIME_SECRET": "docker-dind-integration-secret",
		},
		Labels: map[string]string{
			"test":          "docker-driver-dind",
			"sandbox_image": model.SandboxImageDeveloper,
		},
	})
	if err != nil {
		t.Fatalf("CreateSandbox: %v", err)
	}
	t.Cleanup(func() {
		_ = driver.DeleteSandbox(context.WithoutCancel(ctx), info.ExternalID)
	})

	waitForCommandOutput(t, ctx, driver, info.ExternalID, "systemctl is-active hivy-docker", "active")
	waitForCommandOutput(t, ctx, driver, info.ExternalID, "docker info --format '{{.ServerVersion}}' >/dev/null && echo ready", "ready")
	waitForCommandOutput(t, ctx, driver, info.ExternalID, "docker compose version >/dev/null && echo ready", "ready")

	output, err := driver.ExecuteCommandWithTimeout(ctx, info.ExternalID, nestedComposeSmokeCommand(), 2*time.Minute)
	if err != nil {
		t.Fatalf("nested docker compose smoke: %v output=%q", err, output)
	}
	if !strings.HasSuffix(strings.TrimSpace(output), "compose-ok") {
		t.Fatalf("nested compose response = %q, want compose-ok suffix", strings.TrimSpace(output))
	}
}

func nestedComposeSmokeCommand() string {
	mainSource := `package main

import (
	"net/http"
)

func main() {
	_ = http.ListenAndServe(":8080", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("compose-ok"))
	}))
}
`
	dockerfile := "FROM scratch\nCOPY server /server\nENTRYPOINT [\"/server\"]\n"
	compose := "services:\n  smoke:\n    build: .\n    ports:\n      - \"18080:8080\"\n"
	encode := func(value string) string {
		return base64.StdEncoding.EncodeToString([]byte(value))
	}
	return fmt.Sprintf(`set -eu
work=/tmp/hivy-nested-compose
rm -rf "$work"
mkdir -p "$work"
printf %%s %s | base64 -d >"$work/main.go"
printf %%s %s | base64 -d >"$work/Dockerfile"
printf %%s %s | base64 -d >"$work/compose.yaml"
cd "$work"
CGO_ENABLED=0 mise exec go -- go build -o server main.go
docker compose up -d --build >/tmp/hivy-nested-compose.log
for _ in $(seq 1 40); do
  if response=$(curl -fsS http://127.0.0.1:18080 2>/dev/null); then
    docker compose down >/dev/null
    printf %%s "$response"
    exit 0
  fi
  sleep 0.25
done
docker compose logs
exit 1`, encode(mainSource), encode(dockerfile), encode(compose))
}

func waitForCommandOutput(t *testing.T, ctx context.Context, driver *Driver, externalID, command, want string) {
	t.Helper()

	deadline := time.Now().Add(30 * time.Second)
	var lastOutput string
	var lastErr error
	for time.Now().Before(deadline) {
		lastOutput, lastErr = driver.ExecuteCommand(ctx, externalID, command)
		if lastErr == nil && strings.TrimSpace(lastOutput) == want {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("command %q did not return %q: output=%q err=%v", command, want, strings.TrimSpace(lastOutput), lastErr)
}

func assertHTTPStatusOK(t *testing.T, ctx context.Context, url string) {
	t.Helper()

	client := &http.Client{Timeout: time.Second}
	deadline := time.Now().Add(30 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			t.Fatalf("NewRequest: %v", err)
		}
		resp, err := client.Do(req)
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
			lastErr = fmt.Errorf("status=%d", resp.StatusCode)
		} else {
			lastErr = err
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("GET %s did not return 200: %v", url, lastErr)
}
