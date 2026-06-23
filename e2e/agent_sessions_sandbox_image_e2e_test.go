package e2e

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	dockerclient "github.com/docker/docker/client"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sandbox"
)

func expectedAgentSessionsSandboxRuntimeImage(profile string) string {
	tag := strings.TrimSpace(os.Getenv("HIVY_SANDBOXES_RUNTIME_IMAGE_TAG"))
	if tag == "" {
		tag = "runtime"
	}
	return sandbox.AgentRuntimeImageRef(&config.Config{SandboxesRuntimeImageTag: tag}, profile)
}

func assertAgentSessionsDockerContainerImage(t *testing.T, ctx context.Context, label, externalID, expectedImage string) {
	t.Helper()
	if strings.TrimSpace(externalID) == "" {
		t.Fatalf("%s sandbox external_id is empty", label)
	}
	inspectCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		t.Fatalf("%s create docker client: %v", label, err)
	}
	defer cli.Close()
	info, err := cli.ContainerInspect(inspectCtx, externalID)
	if err != nil {
		t.Fatalf("%s docker inspect failed for external_id=%s: %v", label, externalID, err)
	}
	if info.Config == nil {
		t.Fatalf("%s docker inspect missing container config for external_id=%s", label, externalID)
	}
	if info.Config.Image != expectedImage {
		t.Fatalf("%s docker image = %q, want %q", label, info.Config.Image, expectedImage)
	}
	t.Logf("%s docker image=%s external_id=%s", label, info.Config.Image, externalID)
}

func assertAgentSessionsAgentSandboxImage(t *testing.T, label string, agent agentSessionsAgentListItem, expectedProfile string) {
	t.Helper()
	if agent.SandboxImage != expectedProfile {
		t.Fatalf("%s agent sandbox_image=%q want %q", label, agent.SandboxImage, expectedProfile)
	}
}

func defaultAgentSessionsSandboxRuntimeImage() string {
	return expectedAgentSessionsSandboxRuntimeImage(model.SandboxImageDefault)
}

func developerAgentSessionsSandboxRuntimeImage() string {
	return expectedAgentSessionsSandboxRuntimeImage(model.SandboxImageDeveloper)
}
