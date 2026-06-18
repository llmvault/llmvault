package microsandbox

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/usehivy/hivy/internal/sandbox"
)

const liveE2EControlURL = "https://msb.usehivy.com"
const defaultLiveE2EImageTag = "v3.1.18-amd64"

func TestLiveProviderSnapshotE2E(t *testing.T) {
	if os.Getenv("HIVY_MICROSANDBOX_LIVE_E2E") != "1" {
		t.Skip("set HIVY_MICROSANDBOX_LIVE_E2E=1 to run live Microsandbox provider E2E")
	}
	controlURL := strings.TrimRight(os.Getenv("HIVY_MICROSANDBOX_CONTROL_URL"), "/")
	if controlURL != liveE2EControlURL {
		t.Fatalf("HIVY_MICROSANDBOX_CONTROL_URL must be %s for live E2E, got %q", liveE2EControlURL, controlURL)
	}
	apiToken := strings.TrimSpace(os.Getenv("HIVY_MICROSANDBOX_API_TOKEN"))
	if apiToken == "" {
		t.Fatal("HIVY_MICROSANDBOX_API_TOKEN is required")
	}

	imageTag := strings.TrimSpace(os.Getenv("HIVY_MICROSANDBOX_LIVE_E2E_IMAGE_TAG"))
	if imageTag == "" {
		imageTag = defaultLiveE2EImageTag
	}

	images := []struct {
		name string
		ref  string
	}{
		{name: "runtime", ref: "ghcr.io/usehivy/hivy-sandboxes-runtime:" + imageTag},
		{name: "developers", ref: "ghcr.io/usehivy/hivy-sandboxes-runtime-developers:" + imageTag},
	}
	for _, image := range images {
		t.Run(image.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
			defer cancel()

			driver, err := NewDriver(Config{
				ControlURL:   controlURL,
				APIToken:     apiToken,
				RuntimeImage: image.ref,
			})
			if err != nil {
				t.Fatalf("NewDriver: %v", err)
			}
			if err := driver.Validate(ctx); err != nil {
				t.Fatalf("Validate: %v", err)
			}

			marker := fmt.Sprintf("%s-%d", image.name, time.Now().UnixNano())
			templateID, err := driver.BuildTemplateWithLogs(ctx, sandbox.TemplateBuildRequest{
				Name:          "live-e2e-" + image.name,
				OrgID:         "org_live_e2e",
				BaseImage:     image.ref,
				CPU:           2,
				Memory:        4,
				Disk:          60,
				BuildCommands: liveE2ETemplateCommands(image.name, marker),
			}, nil)
			if err != nil {
				t.Fatalf("BuildTemplateWithLogs: %v", err)
			}
			defer func() {
				if err := driver.DeleteTemplate(context.Background(), templateID); err != nil {
					t.Logf("cleanup DeleteTemplate(%s): %v", templateID, err)
				}
			}()

			status, err := driver.GetTemplateStatus(ctx, templateID)
			if err != nil {
				t.Fatalf("GetTemplateStatus: %v", err)
			}
			if status.State != "ready" {
				t.Fatalf("template state = %q, want ready; error=%q", status.State, status.ErrorMsg)
			}
			logs, err := driver.GetTemplateLogs(ctx, templateID)
			if err != nil {
				t.Fatalf("GetTemplateLogs: %v", err)
			}
			if !strings.Contains(logs, marker) {
				t.Fatalf("template logs do not contain marker %q; logs tail: %q", marker, tail(logs, 1000))
			}

			info, err := driver.CreateSandbox(ctx, sandbox.CreateSandboxOpts{
				Name:        "live-e2e-from-" + image.name,
				TemplateRef: templateID,
				Labels:      map[string]string{"org_id": "org_live_e2e", "purpose": "live-provider-e2e"},
				EnvVars:     map[string]string{"HIVY_MICROSANDBOX_E2E_MARKER": marker},
				CPU:         2,
				Memory:      4,
				Disk:        60,
			})
			if err != nil {
				t.Fatalf("CreateSandbox: %v", err)
			}
			defer func() {
				if err := driver.DeleteSandbox(context.Background(), info.ExternalID); err != nil {
					t.Logf("cleanup DeleteSandbox(%s): %v", info.ExternalID, err)
				}
			}()

			out, err := driver.ExecuteCommand(ctx, info.ExternalID, liveE2EVerifyCommand(image.name, marker))
			if err != nil {
				t.Fatalf("ExecuteCommand: %v\n%s", err, out)
			}
			if err := driver.StopSandbox(ctx, info.ExternalID); err != nil {
				t.Fatalf("StopSandbox: %v", err)
			}
			if err := driver.StartSandbox(ctx, info.ExternalID); err != nil {
				t.Fatalf("StartSandbox: %v", err)
			}
			out, err = driver.ExecuteCommand(ctx, info.ExternalID, fmt.Sprintf("test \"$(cat /opt/hivy-e2e/marker.txt)\" = %q", marker))
			if err != nil {
				t.Fatalf("ExecuteCommand after restart: %v\n%s", err, out)
			}
		})
	}
}

func liveE2ETemplateCommands(imageName, marker string) []string {
	commands := []string{
		fmt.Sprintf("set -eu; mkdir -p /opt/hivy-e2e; printf %%s %q > /opt/hivy-e2e/marker.txt; echo marker=%s", marker, marker),
		"set -eu; node --version >/opt/hivy-e2e/node-version.txt; npm --version >/opt/hivy-e2e/npm-version.txt; git --version >/opt/hivy-e2e/git-version.txt; command -v browser >/opt/hivy-e2e/browser-path.txt; command -v agent-browser >/opt/hivy-e2e/agent-browser-path.txt; browser doctor --offline --quick >/opt/hivy-e2e/browser-doctor.txt",
	}
	if imageName == "developers" {
		commands = append(commands, "set -eu; docker info >/opt/hivy-e2e/docker-info.txt; docker compose version >/opt/hivy-e2e/docker-compose-version.txt")
	}
	return commands
}

func liveE2EVerifyCommand(imageName, marker string) string {
	cmd := fmt.Sprintf("set -eu; test \"$(cat /opt/hivy-e2e/marker.txt)\" = %q; node --version >/tmp/node-version.txt; npm --version >/tmp/npm-version.txt; git --version >/tmp/git-version.txt; command -v browser >/tmp/browser-path.txt; command -v agent-browser >/tmp/agent-browser-path.txt; browser doctor --offline --quick >/tmp/browser-doctor.txt", marker)
	if imageName == "developers" {
		cmd += "; docker info >/tmp/docker-info.txt; docker compose version >/tmp/docker-compose-version.txt"
	}
	return cmd
}

func tail(value string, n int) string {
	if len(value) <= n {
		return value
	}
	return value[len(value)-n:]
}
