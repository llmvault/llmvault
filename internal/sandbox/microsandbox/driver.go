package microsandbox

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/usehivy/hivy/internal/agentruntime"
	msbapi "github.com/usehivy/hivy/internal/microsandbox/api"
	"github.com/usehivy/hivy/internal/sandbox"
)

type Config struct {
	ControlURL   string
	APIToken     string
	RuntimePort  int
	RuntimeImage string
}

type Driver struct {
	controlURL          string
	apiToken            string
	defaultPreviewPorts []int
	runtimePort         int
	runtimeImage        string
	http                *http.Client
}

var agentRuntimeInit = map[string]any{
	"cmd":  "/usr/local/bin/hivy-runtime-entrypoint",
	"args": []string{"/usr/local/bin/hivy-sandboxes-runtime"},
}

func NewDriver(cfg Config) (*Driver, error) {
	runtimePort := cfg.RuntimePort
	if runtimePort == 0 {
		runtimePort = sandbox.AgentSandboxPort
	}
	return &Driver{
		controlURL:          strings.TrimRight(strings.TrimSpace(cfg.ControlURL), "/"),
		apiToken:            strings.TrimSpace(cfg.APIToken),
		defaultPreviewPorts: msbapi.DefaultPreviewPorts(),
		runtimePort:         runtimePort,
		runtimeImage:        strings.TrimSpace(cfg.RuntimeImage),
		http:                &http.Client{Timeout: 35 * time.Minute},
	}, nil
}

func (d *Driver) ID() string { return sandbox.ProviderMicrosandbox }

func (d *Driver) Validate(ctx context.Context) error {
	if d.controlURL == "" {
		return fmt.Errorf("HIVY_MICROSANDBOX_CONTROL_URL is required")
	}
	if d.apiToken == "" {
		return fmt.Errorf("HIVY_MICROSANDBOX_CONTROL_API_TOKEN is required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.controlURL+"/health", nil)
	if err != nil {
		return err
	}
	resp, err := d.http.Do(req)
	if err != nil {
		return fmt.Errorf("microsandbox health: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("microsandbox health returned %d", resp.StatusCode)
	}
	return nil
}

func (d *Driver) RuntimeLayout() sandbox.RuntimeLayout {
	return sandbox.RuntimeLayout{
		AgentRepoDir:     "/workspace/repos",
		WorkspaceRepoDir: "/workspace/repos",
	}
}

func (d *Driver) CreateSandbox(ctx context.Context, opts sandbox.CreateSandboxOpts) (*sandbox.SandboxInfo, error) {
	templateRef := strings.TrimSpace(opts.TemplateRef)
	imageRef := templateRef
	templateID := ""
	if isTemplateRef(templateRef) {
		templateID = templateRef
		imageRef = ""
	}
	if imageRef == "" && templateID == "" {
		imageRef = d.runtimeImage
	}
	if imageRef == "" && templateID == "" {
		return nil, fmt.Errorf("microsandbox: image ref is required")
	}
	body := map[string]any{
		"org_id":        opts.Labels["org_id"],
		"name":          opts.Name,
		"image_ref":     imageRef,
		"template_id":   templateID,
		"size":          msbapi.DefaultSize,
		"cpu":           opts.CPU,
		"memory_mb":     opts.Memory * 1024,
		"disk_gb":       opts.Disk,
		"env":           opts.EnvVars,
		"metadata":      opts.Labels,
		"preview_ports": d.previewPorts(opts.ExposedPorts),
		"init":          agentRuntimeInit,
	}
	var out createSandboxResponse
	if err := d.post(ctx, "/v1/sandboxes", body, &out); err != nil {
		return nil, err
	}
	return &sandbox.SandboxInfo{ExternalID: out.ID, Status: sandbox.StatusRunning}, nil
}

func (d *Driver) CreateWarmSlot(ctx context.Context, opts sandbox.WarmSlotCreateOpts) (*sandbox.WarmSlotInfo, error) {
	imageRef := strings.TrimSpace(opts.RuntimeImage)
	if imageRef == "" {
		imageRef = d.runtimeImage
	}
	if imageRef == "" {
		return nil, fmt.Errorf("microsandbox: warm slot image ref is required")
	}
	port := opts.RuntimePort
	if port == 0 {
		port = d.runtimePort
	}
	metadata := map[string]string{
		"harness":       "agent-sandbox-warm-pool",
		"provider":      sandbox.ProviderMicrosandbox,
		"mode":          opts.Mode,
		"sandbox_image": opts.ImageKind,
		"sandbox_size":  opts.SandboxSize,
	}
	for key, value := range opts.Labels {
		metadata[key] = value
	}
	body := map[string]any{
		"org_id":        "warm-pool",
		"name":          opts.Name,
		"image_ref":     imageRef,
		"template_id":   "",
		"size":          msbapi.DefaultSize,
		"cpu":           opts.CPU,
		"memory_mb":     opts.Memory * 1024,
		"disk_gb":       opts.Disk,
		"env":           d.warmSlotEnv(opts, port),
		"metadata":      metadata,
		"preview_ports": d.previewPorts(nil),
		"init":          agentRuntimeInit,
	}
	var out createSandboxResponse
	if err := d.post(ctx, "/v1/sandboxes", body, &out); err != nil {
		return nil, err
	}
	endpoint, err := d.GetEndpoint(ctx, out.ID, port)
	if err != nil {
		_ = d.DeleteSandbox(context.WithoutCancel(ctx), out.ID)
		return nil, fmt.Errorf("get warm slot runtime endpoint: %w", err)
	}
	return &sandbox.WarmSlotInfo{
		ExternalID:  out.ID,
		EndpointURL: endpoint,
		RuntimePort: port,
	}, nil
}

func (d *Driver) warmSlotEnv(opts sandbox.WarmSlotCreateOpts, port int) map[string]string {
	env := map[string]string{
		agentruntime.AgentEnvRuntimeSecret:   opts.RuntimeSecret,
		agentruntime.AgentEnvRuntimeBindAddr: fmt.Sprintf("0.0.0.0:%d", port),
		agentruntime.AgentEnvWorkspaceRoot:   "/workspace",
		agentruntime.AgentEnvDBPath:          "/app/data/hivy-sandboxes-runtime.db",
		"PORT":                               fmt.Sprintf("%d", port),
	}
	for key, value := range opts.EnvVars {
		env[key] = value
	}
	return env
}

func (d *Driver) UsesWarmPool() bool { return true }

func (d *Driver) previewPorts(exposedPorts []int) []int {
	if len(exposedPorts) == 0 {
		return append([]int(nil), d.defaultPreviewPorts...)
	}
	return uniqueValidPorts(append(append([]int(nil), exposedPorts...), d.runtimePort))
}

func uniqueValidPorts(ports []int) []int {
	out := make([]int, 0, len(ports))
	seen := map[int]struct{}{}
	for _, port := range ports {
		if port <= 0 || port > 65535 {
			continue
		}
		if _, ok := seen[port]; ok {
			continue
		}
		seen[port] = struct{}{}
		out = append(out, port)
	}
	return out
}

func (d *Driver) StartSandbox(ctx context.Context, externalID string) error {
	return d.post(ctx, "/v1/sandboxes/"+externalID+"/start", nil, nil)
}

func (d *Driver) StopSandbox(ctx context.Context, externalID string) error {
	return d.post(ctx, "/v1/sandboxes/"+externalID+"/stop", nil, nil)
}

func (d *Driver) ArchiveSandbox(context.Context, string) error {
	return fmt.Errorf("microsandbox archive sandbox: %w", sandbox.ErrUnsupported)
}

func (d *Driver) DeleteSandbox(ctx context.Context, externalID string) error {
	return d.do(ctx, http.MethodDelete, "/v1/sandboxes/"+externalID, nil, nil)
}

func (d *Driver) GetStatus(ctx context.Context, externalID string) (sandbox.SandboxStatus, error) {
	var out sandboxResponse
	if err := d.do(ctx, http.MethodGet, "/v1/sandboxes/"+externalID, nil, &out); err != nil {
		return sandbox.StatusError, err
	}
	return mapStatus(out.Status), nil
}

func (d *Driver) GetEndpoint(ctx context.Context, externalID string, port int) (string, error) {
	if port == 0 {
		port = d.runtimePort
	}
	var out struct {
		URL string `json:"url"`
	}
	if err := d.post(ctx, "/v1/sandboxes/"+externalID+"/runtime-endpoints", map[string]any{
		"port": port,
	}, &out); err != nil {
		return "", err
	}
	return out.URL, nil
}

func (d *Driver) SetAutoStop(context.Context, string, int) error { return nil }

func (d *Driver) SetAutoArchive(context.Context, string, int) error { return nil }

func (d *Driver) ExecuteCommand(ctx context.Context, externalID string, command string) (string, error) {
	return d.ExecuteCommandWithTimeout(ctx, externalID, command, 2*time.Minute)
}

func (d *Driver) ExecuteCommandWithTimeout(ctx context.Context, externalID string, command string, timeout time.Duration) (string, error) {
	var out struct {
		Stdout   string `json:"stdout"`
		Stderr   string `json:"stderr"`
		ExitCode int    `json:"exit_code"`
	}
	if err := d.post(ctx, "/v1/sandboxes/"+externalID+"/exec", map[string]any{
		"command":         command,
		"timeout_seconds": int(timeout.Seconds()),
	}, &out); err != nil {
		return "", err
	}
	if out.ExitCode != 0 {
		return out.Stdout + out.Stderr, fmt.Errorf("microsandbox command exited with code %d", out.ExitCode)
	}
	return out.Stdout + out.Stderr, nil
}

func (d *Driver) GetResourceUsage(context.Context, string) (*sandbox.ResourceUsage, error) {
	return &sandbox.ResourceUsage{}, nil
}
