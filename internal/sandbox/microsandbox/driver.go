package microsandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/usehivy/hivy/internal/sandbox"
)

const defaultRuntimeEndpointTTLSeconds = 3600

type Config struct {
	ControlURL          string
	APIToken            string
	DefaultPreviewPorts []int
	RuntimePort         int
	RuntimeImage        string
}

type Driver struct {
	controlURL          string
	apiToken            string
	defaultPreviewPorts []int
	runtimePort         int
	runtimeImage        string
	http                *http.Client
}

func NewDriver(cfg Config) (*Driver, error) {
	ports := append([]int(nil), cfg.DefaultPreviewPorts...)
	if len(ports) == 0 {
		ports = []int{3000, 3001, 5173, 7080, 8000, 8080}
	}
	runtimePort := cfg.RuntimePort
	if runtimePort == 0 {
		runtimePort = sandbox.EmployeeSandboxPort
	}
	return &Driver{
		controlURL:          strings.TrimRight(strings.TrimSpace(cfg.ControlURL), "/"),
		apiToken:            strings.TrimSpace(cfg.APIToken),
		defaultPreviewPorts: ports,
		runtimePort:         runtimePort,
		runtimeImage:        strings.TrimSpace(cfg.RuntimeImage),
		http:                &http.Client{Timeout: 2 * time.Minute},
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
		AgentRepoDir:    "/workspace/repos",
		EmployeeRepoDir: "/workspace/repos",
	}
}

func (d *Driver) CreateSandbox(ctx context.Context, opts sandbox.CreateSandboxOpts) (*sandbox.SandboxInfo, error) {
	imageRef := strings.TrimSpace(opts.TemplateRef)
	snapshotID := ""
	if imageRef != "" && !strings.Contains(imageRef, "/") && !strings.Contains(imageRef, ":") {
		snapshotID = imageRef
		imageRef = d.runtimeImage
	}
	if imageRef == "" {
		imageRef = d.runtimeImage
	}
	if imageRef == "" {
		return nil, fmt.Errorf("microsandbox: image ref is required")
	}
	body := map[string]any{
		"org_id":        opts.Labels["org_id"],
		"name":          opts.Name,
		"image_ref":     imageRef,
		"snapshot_id":   snapshotID,
		"cpu":           opts.CPU,
		"memory_mb":     opts.Memory * 1024,
		"disk_gb":       opts.Disk,
		"env":           opts.EnvVars,
		"metadata":      opts.Labels,
		"preview_ports": d.defaultPreviewPorts,
	}
	var out createSandboxResponse
	if err := d.post(ctx, "/v1/sandboxes", body, &out); err != nil {
		return nil, err
	}
	return &sandbox.SandboxInfo{ExternalID: out.ID, Status: sandbox.StatusRunning}, nil
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
		"port":        port,
		"ttl_seconds": defaultRuntimeEndpointTTLSeconds,
	}, &out); err != nil {
		return "", err
	}
	return out.URL, nil
}

func (d *Driver) BuildTemplate(ctx context.Context, opts sandbox.TemplateBuildRequest) (string, error) {
	return d.BuildTemplateWithLogs(ctx, opts, nil)
}

func (d *Driver) BuildTemplateWithLogs(ctx context.Context, opts sandbox.TemplateBuildRequest, _ func(string)) (string, error) {
	baseImage := strings.TrimSpace(opts.BaseImage)
	if baseImage == "" {
		baseImage = d.runtimeImage
	}
	if baseImage == "" {
		return "", fmt.Errorf("microsandbox: base image is required")
	}
	var out snapshotResponse
	if err := d.post(ctx, "/v1/snapshots", map[string]any{
		"org_id":         "system",
		"name":           opts.Name,
		"base_image_ref": baseImage,
		"commands":       opts.BuildCommands,
		"cpu":            opts.CPU,
		"memory_mb":      opts.Memory * 1024,
		"disk_gb":        opts.Disk,
	}, &out); err != nil {
		return "", err
	}
	return out.ID, nil
}

func (d *Driver) GetTemplateStatus(ctx context.Context, externalID string) (*sandbox.TemplateBuildStatus, error) {
	var out snapshotResponse
	if err := d.do(ctx, http.MethodGet, "/v1/snapshots/"+externalID, nil, &out); err != nil {
		return nil, err
	}
	return &sandbox.TemplateBuildStatus{State: out.Status, ErrorMsg: out.ErrorMessage}, nil
}

func (d *Driver) GetTemplateLogs(ctx context.Context, externalID string) (string, error) {
	var out snapshotResponse
	if err := d.do(ctx, http.MethodGet, "/v1/snapshots/"+externalID, nil, &out); err != nil {
		return "", err
	}
	return out.Logs, nil
}

func (d *Driver) DeleteTemplate(context.Context, string) error { return nil }

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

func (d *Driver) post(ctx context.Context, path string, in, out any) error {
	return d.do(ctx, http.MethodPost, path, in, out)
}

func (d *Driver) do(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		raw, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, d.controlURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+d.apiToken)
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := d.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return sandbox.ErrSandboxNotFound
	}
	if resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("microsandbox returned %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

type createSandboxResponse struct {
	ID     string `json:"ID"`
	Status string `json:"Status"`
}

type sandboxResponse struct {
	ID     string `json:"ID"`
	Status string `json:"Status"`
}

type snapshotResponse struct {
	ID           string `json:"ID"`
	Status       string `json:"Status"`
	Logs         string `json:"Logs"`
	ErrorMessage string `json:"ErrorMessage"`
}

func mapStatus(status string) sandbox.SandboxStatus {
	switch strings.ToLower(status) {
	case "running":
		return sandbox.StatusRunning
	case "stopped":
		return sandbox.StatusStopped
	case "creating":
		return sandbox.StatusCreating
	case "error":
		return sandbox.StatusError
	default:
		return sandbox.StatusError
	}
}
