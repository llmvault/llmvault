package tasks

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/usehivy/hivy/internal/sandbox"
	"github.com/usehivy/hivy/internal/storage"
)

type agentUpgradeProvider struct {
	mu           sync.Mutex
	endpoint     string
	failBackup   bool
	failCreate   bool
	failRestore  bool
	failSync     bool
	created      []sandbox.CreateSandboxOpts
	started      []string
	stopped      []string
	deleted      []string
	commands     []string
	nextExternal int
	onCreate     func() // optional hook invoked at the start of CreateSandbox
}

func (p *agentUpgradeProvider) ID() string { return sandbox.ProviderDaytona }

func (p *agentUpgradeProvider) Validate(context.Context) error { return nil }

func (p *agentUpgradeProvider) RuntimeLayout() sandbox.RuntimeLayout {
	return sandbox.RuntimeLayout{
		AgentRepoDir:     "/home/daytona/repos",
		WorkspaceRepoDir: "/workspace/repos",
	}
}

func (p *agentUpgradeProvider) CreateSandbox(_ context.Context, opts sandbox.CreateSandboxOpts) (*sandbox.SandboxInfo, error) {
	p.mu.Lock()
	hook := p.onCreate
	p.mu.Unlock()
	if hook != nil {
		hook()
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.failCreate {
		return nil, errors.New("create failed")
	}
	p.nextExternal++
	p.created = append(p.created, opts)
	return &sandbox.SandboxInfo{ExternalID: fmt.Sprintf("new-external-%d", p.nextExternal), Status: sandbox.StatusRunning}, nil
}

func (p *agentUpgradeProvider) StartSandbox(_ context.Context, externalID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.started = append(p.started, externalID)
	return nil
}

func (p *agentUpgradeProvider) StopSandbox(_ context.Context, externalID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stopped = append(p.stopped, externalID)
	return nil
}

func (p *agentUpgradeProvider) ArchiveSandbox(context.Context, string) error { return nil }

func (p *agentUpgradeProvider) DeleteSandbox(_ context.Context, externalID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.deleted = append(p.deleted, externalID)
	return nil
}

func (p *agentUpgradeProvider) GetStatus(context.Context, string) (sandbox.SandboxStatus, error) {
	return sandbox.StatusRunning, nil
}

func (p *agentUpgradeProvider) GetEndpoint(context.Context, string, int) (string, error) {
	return p.endpoint, nil
}

func (p *agentUpgradeProvider) BuildTemplate(context.Context, sandbox.TemplateBuildRequest) (string, error) {
	return "", nil
}

func (p *agentUpgradeProvider) BuildTemplateWithLogs(context.Context, sandbox.TemplateBuildRequest, func(string)) (string, error) {
	return "", nil
}

func (p *agentUpgradeProvider) GetTemplateStatus(context.Context, string) (*sandbox.TemplateBuildStatus, error) {
	return &sandbox.TemplateBuildStatus{State: "ready"}, nil
}

func (p *agentUpgradeProvider) GetTemplateLogs(context.Context, string) (string, error) {
	return "", nil
}

func (p *agentUpgradeProvider) DeleteTemplate(context.Context, string) error { return nil }

func (p *agentUpgradeProvider) SetAutoStop(context.Context, string, int) error {
	return nil
}

func (p *agentUpgradeProvider) SetAutoArchive(context.Context, string, int) error {
	return nil
}

func (p *agentUpgradeProvider) ExecuteCommand(ctx context.Context, externalID string, command string) (string, error) {
	return p.ExecuteCommandWithTimeout(ctx, externalID, command, 0)
}

func (p *agentUpgradeProvider) ExecuteCommandWithTimeout(_ context.Context, _ string, command string, _ time.Duration) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.commands = append(p.commands, command)
	switch {
	case strings.Contains(command, "VACUUM main INTO"):
		if p.failBackup {
			return "", errors.New("backup failed")
		}
		return `{"sha256":"` + strings.Repeat("a", 64) + `","bytes":12}`, nil
	case strings.Contains(command, "install -m 600"):
		if p.failRestore {
			return "", errors.New("restore failed")
		}
		return `{"status":"ok"}`, nil
	default:
		return "", nil
	}
}

func (p *agentUpgradeProvider) GetResourceUsage(context.Context, string) (*sandbox.ResourceUsage, error) {
	return &sandbox.ResourceUsage{}, nil
}

type fakeAgentUpgradeStore struct {
	size int64
}

func (s fakeAgentUpgradeStore) Head(context.Context, string) (*storage.S3ObjectInfo, error) {
	return &storage.S3ObjectInfo{Size: s.size}, nil
}

func (s fakeAgentUpgradeStore) PresignedURL(context.Context, string, time.Duration) (string, error) {
	return "https://s3.example/backup.db.gz", nil
}

func (s fakeAgentUpgradeStore) PresignedPutURL(context.Context, string, time.Duration) (string, error) {
	return "https://s3.example/upload.db.gz?signature=test", nil
}
