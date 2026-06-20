package model

import "time"

type Runner struct {
	ID               string  `gorm:"primaryKey"`
	Name             string  `gorm:"not null;uniqueIndex"`
	APIURL           string  `gorm:"not null"`
	PreviewBaseURL   string  `gorm:"not null;default:''"`
	AuthTokenHash    []byte  `gorm:"not null"`
	Status           string  `gorm:"not null;index"`
	Drain            bool    `gorm:"not null;default:false"`
	TotalCPU         int     `gorm:"not null"`
	TotalMemoryMB    int     `gorm:"not null"`
	TotalDiskGB      int     `gorm:"not null"`
	ReservedCPU      int     `gorm:"not null;default:0"`
	ReservedMemoryMB int     `gorm:"not null;default:0"`
	ReservedDiskGB   int     `gorm:"not null;default:0"`
	CPUOvercommit    float64 `gorm:"not null;default:1.5"`
	MemoryOvercommit float64 `gorm:"not null;default:1"`
	DiskOvercommit   float64 `gorm:"not null;default:4"`
	MetadataJSON     string  `gorm:"type:text;not null;default:'{}'"`
	LastHeartbeatAt  *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type OrgPreviewSecret struct {
	OrgID              string `gorm:"primaryKey"`
	PasswordCiphertext []byte `gorm:"not null"`
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type Sandbox struct {
	ID                    string `gorm:"primaryKey"`
	OrgID                 string `gorm:"not null;index"`
	RunnerID              string `gorm:"not null;index"`
	Name                  string `gorm:"not null"`
	ImageRef              string `gorm:"not null"`
	Status                string `gorm:"not null;index"`
	CPU                   int    `gorm:"not null"`
	MemoryMB              int    `gorm:"not null"`
	DiskGB                int    `gorm:"not null"`
	MetadataJSON          string `gorm:"type:text;not null;default:'{}'"`
	ErrorMessage          string `gorm:"type:text;not null;default:''"`
	InfraProbeToken       string `gorm:"type:text;not null;default:''"`
	AutoSleepAfterSeconds int    `gorm:"not null;default:0;index"`
	RuntimeBusy           bool   `gorm:"not null;default:false;index"`
	CreatedAt             time.Time
	UpdatedAt             time.Time
	StoppedAt             *time.Time
	LastGatewayActivityAt *time.Time
	LastRuntimeActivityAt *time.Time
	LastWakeAt            *time.Time
	LastWakeError         string `gorm:"type:text;not null;default:''"`
	SleepAfterAt          *time.Time
	RouteGeneration       int64  `gorm:"not null;default:1"`
	ActivityToken         string `gorm:"type:text;not null;default:''"`
}

type SandboxPort struct {
	ID        string `gorm:"primaryKey"`
	SandboxID string `gorm:"not null;index:idx_sandbox_ports_sandbox_guest,unique"`
	GuestPort int    `gorm:"not null;index:idx_sandbox_ports_sandbox_guest,unique"`
	HostPort  int    `gorm:"not null"`
	Protocol  string `gorm:"not null;default:'http'"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Template struct {
	ID                  string `gorm:"primaryKey"`
	OrgID               string `gorm:"not null;index"`
	RunnerID            string `gorm:"not null;index"`
	Name                string `gorm:"not null"`
	BaseImageRef        string `gorm:"not null"`
	Status              string `gorm:"not null;index"`
	ImageRef            string `gorm:"type:text;not null;default:''"`
	ImageDigest         string `gorm:"type:text;not null;default:''"`
	CommandsJSON        string `gorm:"type:text;not null;default:'[]'"`
	Logs                string `gorm:"type:text;not null;default:''"`
	ErrorMessage        string `gorm:"type:text;not null;default:''"`
	ValidationSandboxID string `gorm:"type:text;not null;default:''"`
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type Event struct {
	ID        string `gorm:"primaryKey"`
	Kind      string `gorm:"not null;index"`
	RunnerID  string `gorm:"index"`
	SandboxID string `gorm:"index"`
	Message   string `gorm:"type:text;not null"`
	CreatedAt time.Time
}

func (Runner) TableName() string { return "microsandbox_runners" }

func (OrgPreviewSecret) TableName() string { return "microsandbox_org_preview_secrets" }

func (Sandbox) TableName() string { return "microsandbox_sandboxes" }

func (SandboxPort) TableName() string { return "microsandbox_sandbox_ports" }

func (Template) TableName() string { return "microsandbox_templates" }

func (Event) TableName() string { return "microsandbox_events" }

const (
	RunnerStatusHealthy   = "healthy"
	RunnerStatusUnhealthy = "unhealthy"

	SandboxStatusCreating = "creating"
	SandboxStatusRunning  = "running"
	SandboxStatusStopped  = "stopped"
	SandboxStatusError    = "error"

	TemplateStatusBuilding = "building"
	TemplateStatusReady    = "ready"
	TemplateStatusError    = "error"
)
