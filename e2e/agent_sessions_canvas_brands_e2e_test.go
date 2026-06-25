package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/model"
)

type agentSessionsBrandMutation struct {
	Brand agentSessionsBrand `json:"brand"`
}

type agentSessionsBrandPage struct {
	Data []agentSessionsBrand `json:"data"`
}

type agentSessionsBrand struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Slug        string         `json:"slug"`
	Description string         `json:"description"`
	IsDefault   bool           `json:"is_default"`
	Colors      map[string]any `json:"colors"`
	Voice       map[string]any `json:"voice"`
}

func TestAgentSessionsCanvasBrandsCLIE2E(t *testing.T) {
	loadEnv(t)
	if os.Getenv("HIVY_AGENT_SESSIONS_E2E") != "1" {
		t.Skip("set HIVY_AGENT_SESSIONS_E2E=1 to run against the live compose stack")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()

	apiBase := agentSessionsBaseURL("HIVY_API_BASE_URL", "HIVY_COMPOSE_API_PORT", "8080")
	requireAgentSessionsHealthy(t, ctx, apiBase, "api")

	runID := strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	password := "agent-sessions-canvas-brands-password"
	ownerEmail := "agent-sessions-canvas-brands-" + runID + "@example.com"
	ownerAuth := agentSessionsRegister(t, ctx, apiBase, ownerEmail, password, "Canvas Brands "+runID)
	orgID := ownerAuth.Orgs[0].ID
	ownerToken := ownerAuth.AccessToken

	agents := agentSessionsListAgents(t, ctx, apiBase, ownerToken, orgID)
	defaultAgent := findDefaultAgent(t, agents)
	assertAgentSessionsAgentSandboxImage(t, "canvas brands Hivy", defaultAgent, model.SandboxImageDefault)

	sb := agentSessionsLaunchCanvasRuntime(t, ctx, apiBase, orgID, defaultAgent.ID, runID)
	t.Cleanup(func() {
		agentSessionsDeleteSandbox(t, ctx, apiBase, ownerToken, orgID, sb.ID.String())
	})
	assertAgentSessionsDockerContainer(t, ctx, "canvas brands runtime", sb)
	assertAgentSessionsDockerContainerImage(t, ctx, "canvas brands Hivy", sb.ExternalID, defaultAgentSessionsSandboxRuntimeImage())

	initialList := decodeCanvasBrandPage(t, agentSessionsDockerCanvas(t, ctx, sb.ExternalID, "brands", "list"))
	t.Logf("initial canvas brands count=%d", len(initialList.Data))

	brandName := "Canvas CLI Brand " + runID
	createJSON := `{"colors":{"tokens":[{"id":"primary","value":"#2463eb","roles":["primary"]}],"semantic":{"primary":"primary"}}}`
	created := decodeCanvasBrandMutation(t, agentSessionsDockerCanvas(t, ctx, sb.ExternalID,
		"brand", "create",
		"--name", brandName,
		"--description", "Created by runtime CLI",
		"--default",
		"--json", createJSON,
	))
	if created.Brand.ID == "" || created.Brand.Name != brandName || !created.Brand.IsDefault {
		t.Fatalf("bad created brand: %+v", created.Brand)
	}

	viewed := decodeCanvasBrandMutation(t, agentSessionsDockerCanvas(t, ctx, sb.ExternalID, "brand", "view", created.Brand.ID))
	if viewed.Brand.ID != created.Brand.ID || viewed.Brand.Name != brandName {
		t.Fatalf("bad viewed brand: %+v", viewed.Brand)
	}

	updated := decodeCanvasBrandMutation(t, agentSessionsDockerCanvas(t, ctx, sb.ExternalID,
		"brands", "update", created.Brand.ID,
		"--json", `{"description":"Updated by runtime CLI","voice":{"personality":["direct","clear"]}}`,
	))
	if updated.Brand.Description != "Updated by runtime CLI" {
		t.Fatalf("updated brand description=%q", updated.Brand.Description)
	}

	finalList := decodeCanvasBrandPage(t, agentSessionsDockerCanvas(t, ctx, sb.ExternalID, "brands", "list"))
	if !brandPageHasID(finalList, created.Brand.ID) {
		t.Fatalf("canvas brands list missing created brand id=%s page=%+v", created.Brand.ID, finalList.Data)
	}

	apiBrand := agentSessionsGetBrand(t, ctx, apiBase, ownerToken, orgID, created.Brand.ID)
	if apiBrand.Name != brandName || apiBrand.Description != "Updated by runtime CLI" {
		t.Fatalf("API brand does not match CLI mutation: %+v", apiBrand)
	}
	semantic, _ := apiBrand.Colors["semantic"].(map[string]any)
	if primary, _ := semantic["primary"].(string); primary != "primary" {
		t.Fatalf("API brand colors missing semantic primary: %+v", apiBrand.Colors)
	}
}

func agentSessionsLaunchCanvasRuntime(t *testing.T, ctx context.Context, apiBase, orgIDRaw, agentIDRaw, runID string) model.Sandbox {
	t.Helper()
	runtimeSecret := "canvas-brands-e2e-" + runID
	sb := agentSessionsCreateCanvasSandboxRow(t, ctx, orgIDRaw, agentIDRaw, runtimeSecret, runID)
	containerName := sb.ExternalID

	runCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	args := []string{
		"run", "-d",
		"--name", containerName,
		"--add-host", "host.docker.internal:host-gateway",
		"-e", "HIVY_CONTROL_PLANE_URL=" + agentSessionsDockerControlPlaneURL(t, apiBase),
		"-e", "HIVY_AGENT_ID=" + agentIDRaw,
		"-e", "HIVY_RUNTIME_SECRET=" + runtimeSecret,
		"-e", "HIVY_RUNTIME_BIND_ADDR=0.0.0.0:7080",
		defaultAgentSessionsSandboxRuntimeImage(),
	}
	out, err := exec.CommandContext(runCtx, "docker", args...).CombinedOutput()
	t.Logf("docker %s output=%s", strings.Join(args, " "), strings.TrimSpace(string(out)))
	if err != nil {
		t.Fatalf("docker %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
	t.Cleanup(func() {
		rmCtx, rmCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer rmCancel()
		_ = exec.CommandContext(rmCtx, "docker", "rm", "-f", containerName).Run()
	})
	return sb
}

func agentSessionsCreateCanvasSandboxRow(t *testing.T, ctx context.Context, orgIDRaw, agentIDRaw, runtimeSecret, runID string) model.Sandbox {
	t.Helper()
	key, err := crypto.NewSymmetricKey(strings.TrimSpace(os.Getenv("HIVY_SANDBOX_ENCRYPTION_KEY")))
	if err != nil {
		t.Fatalf("create sandbox encryption key: %v", err)
	}
	encrypted, err := key.EncryptString(runtimeSecret)
	if err != nil {
		t.Fatalf("encrypt runtime secret: %v", err)
	}
	orgID := uuid.MustParse(orgIDRaw)
	agentID := uuid.MustParse(agentIDRaw)
	now := time.Now()
	containerName := "hivy-canvas-brands-e2e-" + runID
	sb := model.Sandbox{
		ID:                     uuid.New(),
		OrgID:                  &orgID,
		AgentID:                &agentID,
		ProviderID:             "docker",
		ExternalID:             containerName,
		RuntimeURL:             "http://127.0.0.1:0",
		EncryptedRuntimeSecret: encrypted,
		Status:                 "running",
		LastActiveAt:           &now,
	}
	db := agentSessionsOpenDB(t)
	if err := db.WithContext(ctx).Create(&sb).Error; err != nil {
		t.Fatalf("create canvas brands sandbox fixture: %v", err)
	}
	t.Cleanup(func() {
		db.WithContext(context.Background()).Where("id = ?", sb.ID).Delete(&model.Sandbox{})
	})
	return sb
}

func agentSessionsDockerControlPlaneURL(t *testing.T, apiBase string) string {
	t.Helper()
	parsed, err := url.Parse(apiBase)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		t.Fatalf("parse API base URL %q: %v", apiBase, err)
	}
	host, port, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		host = parsed.Hostname()
		port = parsed.Port()
	}
	switch strings.ToLower(host) {
	case "localhost", "127.0.0.1", "::1":
		host = "host.docker.internal"
	}
	if port != "" {
		parsed.Host = net.JoinHostPort(host, port)
	} else {
		parsed.Host = host
	}
	return strings.TrimRight(parsed.String(), "/")
}

func agentSessionsDockerCanvas(t *testing.T, ctx context.Context, containerID string, args ...string) []byte {
	t.Helper()
	execCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	cmdArgs := append([]string{"exec", containerID, "canvas"}, args...)
	cmd := exec.CommandContext(execCtx, "docker", cmdArgs...)
	out, err := cmd.CombinedOutput()
	t.Logf("docker %s output=%s", strings.Join(cmdArgs, " "), strings.TrimSpace(string(out)))
	if err != nil {
		t.Fatalf("docker %s failed: %v\n%s", strings.Join(cmdArgs, " "), err, out)
	}
	return out
}

func decodeCanvasBrandMutation(t *testing.T, raw []byte) agentSessionsBrandMutation {
	t.Helper()
	var out agentSessionsBrandMutation
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode canvas brand mutation: %v\n%s", err, raw)
	}
	return out
}

func decodeCanvasBrandPage(t *testing.T, raw []byte) agentSessionsBrandPage {
	t.Helper()
	var out agentSessionsBrandPage
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode canvas brand page: %v\n%s", err, raw)
	}
	return out
}

func agentSessionsGetBrand(t *testing.T, ctx context.Context, baseURL, token, orgID, brandID string) agentSessionsBrand {
	t.Helper()
	var out agentSessionsBrandMutation
	agentSessionsJSON(t, ctx, "GET", fmt.Sprintf("%s/v1/orgs/current/brands/%s", baseURL, brandID), token, orgID, nil, 200, &out)
	return out.Brand
}

func brandPageHasID(page agentSessionsBrandPage, brandID string) bool {
	for _, brand := range page.Data {
		if brand.ID == brandID {
			return true
		}
	}
	return false
}
