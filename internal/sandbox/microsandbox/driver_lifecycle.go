package microsandbox

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/usehivy/hivy/internal/sandbox"
)

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

// sandboxStateResponse mirrors the control plane's lean /v1/sandboxes/states
// projection.
type sandboxStateResponse struct {
	ID                    string     `json:"id"`
	Status                string     `json:"status"`
	SleepAfterAt          *time.Time `json:"sleep_after_at"`
	LastGatewayActivityAt *time.Time `json:"last_gateway_activity_at"`
	RuntimeBusy           bool       `json:"runtime_busy"`
	LastRuntimeActivityAt *time.Time `json:"last_runtime_activity_at"`
}

// ListSandboxStates returns the control plane's liveness view of every sandbox
// in one batch call, so the reconciler can bulk-correct the Go-API mirror.
func (d *Driver) ListSandboxStates(ctx context.Context) ([]sandbox.SandboxState, error) {
	var out struct {
		Data []sandboxStateResponse `json:"data"`
	}
	if err := d.do(ctx, http.MethodGet, "/v1/sandboxes/states", nil, &out); err != nil {
		return nil, err
	}
	states := make([]sandbox.SandboxState, 0, len(out.Data))
	for _, s := range out.Data {
		states = append(states, sandbox.SandboxState{
			ExternalID:            s.ID,
			Status:                mapStatus(s.Status),
			LastGatewayActivityAt: s.LastGatewayActivityAt,
			RuntimeBusy:           s.RuntimeBusy,
			LastRuntimeActivityAt: s.LastRuntimeActivityAt,
		})
	}
	return states, nil
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

func (d *Driver) SetAutoStop(ctx context.Context, externalID string, intervalMinutes int) error {
	seconds := 0
	if intervalMinutes > 0 {
		seconds = intervalMinutes * 60
	}
	return d.patch(ctx, "/v1/sandboxes/"+externalID+"/policy", map[string]any{
		"auto_sleep_after_seconds": seconds,
	}, nil)
}

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
