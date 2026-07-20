package sandbox

import (
	"context"
	"errors"
	"time"
)

var ErrSandboxNotFound = errors.New("sandbox not found upstream")
var ErrSandboxDraining = errors.New("sandbox is draining")

const (
	ProviderDaytona      = "daytona"
	ProviderDocker       = "docker"
	ProviderMicrosandbox = "microsandbox"
	ProviderRailway      = "railway"
)

// SandboxStatus represents the state of a sandbox.
type SandboxStatus string

const (
	StatusCreating  SandboxStatus = "creating"
	StatusRunning   SandboxStatus = "running"
	StatusStopped   SandboxStatus = "stopped"
	StatusStarting  SandboxStatus = "starting"
	StatusArchived  SandboxStatus = "archived"
	StatusArchiving SandboxStatus = "archiving"
	StatusError     SandboxStatus = "error"
	// StatusDraining marks a sandbox that has accepted a recycle drain signal.
	// It is deliberately non-selectable for new runtime traffic; the control
	// plane polls the runtime until accepted turns and outbound webhooks finish.
	StatusDraining SandboxStatus = "draining"
)

// SandboxState is a lean liveness snapshot for the reconciler: the control
// plane's own view (which may diverge from ours after a gateway-driven wake)
// plus the activity signals downstream idle logic uses.
type SandboxState struct {
	ExternalID            string
	Status                SandboxStatus
	LastGatewayActivityAt *time.Time
	RuntimeBusy           bool
	LastRuntimeActivityAt *time.Time
}

// SandboxStateLister is an optional provider capability: the state of every
// sandbox in one batch call. Unimplementing providers are skipped.
type SandboxStateLister interface {
	ListSandboxStates(ctx context.Context) ([]SandboxState, error)
}

// CreateSandboxOpts configures a new sandbox.
type CreateSandboxOpts struct {
	Name         string            // human-readable name
	TemplateRef  string            // provider template/image reference
	EnvVars      map[string]string // runtime environment variables
	Labels       map[string]string // metadata labels (org_id, sandbox_id, agent_id)
	CPU          int               // CPU cores (0 = provider default)
	Memory       int               // memory in GB (0 = provider default)
	Disk         int               // disk in GB (0 = provider default)
	ExposedPorts []int             // user-facing preview ports to expose
	// HealthCheck overrides the default agent-runtime health probe (/healthz on
	// the runtime port). App sandboxes set this so the wake probe targets the
	// app daemon, which serves /health rather than the agent-runtime /healthz.
	// Nil keeps the agent-runtime default so non-app sandboxes are unaffected.
	HealthCheck *SandboxHealthCheck
}

// SandboxHealthCheck is an explicit HTTP runtime health probe for a sandbox
// port. It overrides the driver's default agent-runtime probe.
type SandboxHealthCheck struct {
	Port           int    // guest port to probe (must be previewable/exposed)
	Path           string // request path, e.g. "/health"
	ExpectedStatus int    // status treated as healthy (0 defaults to 200)
}

// SandboxInfo is returned after creating a sandbox.
type SandboxInfo struct {
	ExternalID string // provider's sandbox identifier
	Status     SandboxStatus
}

// TemplateBuildRequest configures a provider template/image build.
type TemplateBuildRequest struct {
	Name          string   // provider template name
	OrgID         string   // owning org ID; "system" only for platform templates
	BuildCommands []string // commands to run on the base image
	BaseImage     string   // base image to build on top of
	CPU           int      // CPU cores (0 = provider default)
	Memory        int      // memory in GB (0 = provider default)
	Disk          int      // disk in GB (0 = provider default)
}

// TemplateBuildResult tracks template build progress.
type TemplateBuildResult struct {
	ExternalID string
	Ready      bool
	Error      string
}

// TemplateBuildStatus holds the status check result.
type TemplateBuildStatus struct {
	State       string // "building", "ready", "error", "deleting"
	ErrorMsg    string
	ErrorReason string // provider-specific detailed error reason
}

// ResourceUsage is the provider-normalized runtime resource usage for a sandbox.
type ResourceUsage struct {
	MemoryLimitBytes  int64
	MemoryUsedBytes   int64
	MemoryPeakBytes   int64
	CPUQuota          string
	CPUUsageUsec      int64
	CPUThrottledCount int64
	PIDCount          int64
}

type RuntimeLayout struct {
	AgentRepoDir     string
	WorkspaceRepoDir string
}

type WarmSlotCreateOpts struct {
	Name          string
	Mode          string
	ImageKind     string
	SandboxSize   string
	RuntimeImage  string
	RuntimePort   int
	CPU           int
	Memory        int
	Disk          int
	RuntimeSecret string
	EnvVars       map[string]string
	Labels        map[string]string
}

type WarmSlotInfo struct {
	ExternalID  string
	EndpointURL string
	RuntimePort int
}

type WarmSlotProvider interface {
	CreateWarmSlot(ctx context.Context, opts WarmSlotCreateOpts) (*WarmSlotInfo, error)
}

// Provider is the complete startup-time contract a sandbox backend must satisfy.
type Provider interface {
	ID() string
	Validate(ctx context.Context) error
	RuntimeLayout() RuntimeLayout

	CreateSandbox(ctx context.Context, opts CreateSandboxOpts) (*SandboxInfo, error)
	StartSandbox(ctx context.Context, externalID string) error
	StopSandbox(ctx context.Context, externalID string) error
	// ArchiveSandbox moves a stopped sandbox into cold storage. The provider must
	// be able to restore it via StartSandbox. The sandbox must be stopped first.
	ArchiveSandbox(ctx context.Context, externalID string) error
	DeleteSandbox(ctx context.Context, externalID string) error
	GetStatus(ctx context.Context, externalID string) (SandboxStatus, error)

	// Networking — returns the URL to reach a port inside the sandbox.
	GetEndpoint(ctx context.Context, externalID string, port int) (string, error)

	BuildTemplate(ctx context.Context, opts TemplateBuildRequest) (externalID string, err error)
	BuildTemplateWithLogs(ctx context.Context, opts TemplateBuildRequest, onLog func(string)) (externalID string, err error)
	GetTemplateStatus(ctx context.Context, externalID string) (*TemplateBuildStatus, error)
	GetTemplateLogs(ctx context.Context, externalID string) (string, error)
	DeleteTemplate(ctx context.Context, externalID string) error

	// Auto-management. A zero idle timeout disables the policy.
	SetAutoStop(ctx context.Context, externalID string, idleTimeout time.Duration) error
	SetAutoArchive(ctx context.Context, externalID string, intervalMinutes int) error

	// Execution — run a command inside the sandbox.
	ExecuteCommand(ctx context.Context, externalID string, command string) (string, error)
	ExecuteCommandWithTimeout(ctx context.Context, externalID string, command string, timeout time.Duration) (string, error)

	GetResourceUsage(ctx context.Context, externalID string) (*ResourceUsage, error)
}

// WarmPoolCapable is implemented by providers that create sandboxes
// through a warm pool rather than direct CreateSandbox calls.
type WarmPoolCapable interface {
	UsesWarmPool() bool
}

// ErrUnsupported means a lifecycle operation has no provider-side implementation
// (e.g. Railway has no scale-to-zero). Callers must skip the control-plane state
// change rather than persist a state the provider never reached.
var ErrUnsupported = errors.New("operation not supported by sandbox provider")

// RuntimeCommandContext bundles what a provider needs for runtime-based command execution.
type RuntimeCommandContext struct {
	RuntimeURL    string
	RuntimeSecret string
}

// RuntimeCommandExecutor is implemented by providers that cannot execute
// commands via the provider API and route through an HTTP runtime instead.
type RuntimeCommandExecutor interface {
	ExecuteCommandViaRuntime(ctx context.Context, cmdCtx RuntimeCommandContext, command string, timeout time.Duration) (string, error)
}

type RestartableProvider interface {
	RestartSandbox(ctx context.Context, externalID string) error
}
