package docker

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/usehivy/hivy/internal/sandbox"
)

func TestDockerDriverUpgradeSandboxPreservesIdentityEndpointAndWorkspace(t *testing.T) {
	ctx := context.Background()
	driver := newIntegrationDriver(t, ctx)
	imageV1 := buildIntegrationImage(t, ctx, driver, "upgrade-v1", integrationUpgradeDockerfile("v1"))
	imageV2 := buildIntegrationImage(t, ctx, driver, "upgrade-v2", integrationUpgradeDockerfile("v2"))
	labels := map[string]string{"test": "docker-upgrade"}

	info, err := driver.CreateSandbox(ctx, sandbox.CreateSandboxOpts{
		Name:        "hivy-docker-upgrade-integration",
		TemplateRef: imageV1,
		Labels:      labels,
		CPU:         1,
		Memory:      1,
	})
	if err != nil {
		t.Fatalf("CreateSandbox: %v", err)
	}
	t.Cleanup(func() {
		_ = driver.DeleteSandbox(context.Background(), info.ExternalID)
	})

	urlBefore, err := driver.GetEndpoint(ctx, info.ExternalID, 8080)
	if err != nil {
		t.Fatalf("GetEndpoint before upgrade: %v", err)
	}
	assertHTTPBody(t, ctx, urlBefore, "v1")
	marker := "marker-" + fmt.Sprint(time.Now().UnixNano())
	output, err := driver.ExecuteCommand(ctx, info.ExternalID, "printf '"+marker+"' > /workspace/hivy-upgrade-marker && cat /image-version")
	if err != nil {
		t.Fatalf("write marker: %v output=%q", err, output)
	}
	if strings.TrimSpace(output) != "v1" {
		t.Fatalf("image version before upgrade = %q, want v1", output)
	}

	upgraded, err := driver.UpgradeSandbox(ctx, info.ExternalID, sandbox.UpgradeSandboxOpts{
		Name:        "hivy-docker-upgrade-integration",
		TemplateRef: imageV2,
		Labels:      labels,
		CPU:         1,
		Memory:      1,
	})
	if err != nil {
		t.Fatalf("UpgradeSandbox: %v", err)
	}
	if upgraded.ExternalID != info.ExternalID {
		t.Fatalf("external id after upgrade = %q, want %q", upgraded.ExternalID, info.ExternalID)
	}
	urlAfter, err := driver.GetEndpoint(ctx, upgraded.ExternalID, 8080)
	if err != nil {
		t.Fatalf("GetEndpoint after upgrade: %v", err)
	}
	if urlAfter != urlBefore {
		t.Fatalf("endpoint after upgrade = %q, want %q", urlAfter, urlBefore)
	}
	assertHTTPBody(t, ctx, urlAfter, "v2")
	output, err = driver.ExecuteCommand(ctx, upgraded.ExternalID, "cat /workspace/hivy-upgrade-marker && printf '\\n' && cat /image-version")
	if err != nil {
		t.Fatalf("verify after upgrade: %v output=%q", err, output)
	}
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 2 || lines[0] != marker || lines[1] != "v2" {
		t.Fatalf("post-upgrade output = %q, want marker and v2", output)
	}
}

func integrationUpgradeDockerfile(version string) string {
	return `FROM redis:7-alpine
ENTRYPOINT []
RUN echo ` + version + ` > /image-version
EXPOSE 8080
CMD ["/bin/sh", "-lc", "while true; do printf 'HTTP/1.1 200 OK\r\nContent-Length: 3\r\n\r\n` + version + `\n' | nc -l -p 8080; done"]
`
}
