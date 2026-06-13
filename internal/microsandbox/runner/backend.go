package runner

import (
	"context"
	"io"
)

type Backend interface {
	Reconcile(ctx context.Context) (*ReconcileReport, error)
	Status(ctx context.Context) (map[string]any, error)
	CreateSandbox(ctx context.Context, req CreateSandboxRequest) (*CreateSandboxResponse, error)
	StartSandbox(ctx context.Context, sandboxID string) error
	StopSandbox(ctx context.Context, sandboxID string) error
	DeleteSandbox(ctx context.Context, sandboxID string) error
	Exec(ctx context.Context, sandboxID, command string, timeoutSeconds int) (*ExecResponse, error)
	Logs(ctx context.Context, sandboxID string, w io.Writer) error
	Proxy(ctx context.Context, sandboxID string, guestPort int, w io.Writer, r io.Reader) error
	ProxyURL(ctx context.Context, sandboxID string, guestPort int) (string, error)
	CreateSnapshot(ctx context.Context, req CreateSnapshotRequest) (*CreateSnapshotResponse, error)
}

type ReconcileReport struct {
	Sandboxes int `json:"sandboxes"`
	Ports     int `json:"ports"`
	Skipped   int `json:"skipped"`
}

type CreateSandboxRequest struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	ImageRef     string            `json:"image_ref"`
	SnapshotID   string            `json:"snapshot_id"`
	CPU          int               `json:"cpu"`
	MemoryMB     int               `json:"memory_mb"`
	DiskGB       int               `json:"disk_gb"`
	Env          map[string]string `json:"env"`
	Labels       map[string]string `json:"labels"`
	PreviewPorts []int             `json:"preview_ports"`
}

type CreateSandboxResponse struct {
	ID    string        `json:"id"`
	Ports []PortBinding `json:"ports"`
}

type PortBinding struct {
	GuestPort int `json:"guest_port"`
	HostPort  int `json:"host_port"`
}

type ExecResponse struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
}

type CreateSnapshotRequest struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	BaseImageRef string            `json:"base_image_ref"`
	Commands     []string          `json:"commands"`
	Env          map[string]string `json:"env"`
	CPU          int               `json:"cpu"`
	MemoryMB     int               `json:"memory_mb"`
	DiskGB       int               `json:"disk_gb"`
}

type CreateSnapshotResponse struct {
	ID          string `json:"id"`
	ArtifactURL string `json:"artifact_url"`
	Logs        string `json:"logs"`
}
