package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/sandbox"
)

type agentSandboxUpgradeHarness struct {
	db       *gorm.DB
	encKey   *crypto.SymmetricKey
	runtime  *agentSandboxUpgradeRuntime
	provider *agentSandboxUpgradeProvider
	handler  *AgentSandboxUpgradeHandler
}

func newAgentSandboxUpgradeHarness(t *testing.T) *agentSandboxUpgradeHarness {
	t.Helper()
	db := connectTestDB(t)
	encKey, err := crypto.NewSymmetricKey("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=")
	if err != nil {
		t.Fatalf("create enc key: %v", err)
	}
	runtime := newAgentSandboxUpgradeRuntime(t)
	cfg := &config.Config{
		SandboxProviderID:        sandbox.ProviderMicrosandbox,
		SandboxesRuntimeImageTag: "upgrade-test-amd64",
		APIWebhookBaseURL:        "https://api.example.test",
		ProxyHost:                "https://proxy.example.test",
		MCPBaseURL:               "https://mcp.example.test",
		JWTSigningKey:            "upgrade-test-signing-key",
	}
	provider := &agentSandboxUpgradeProvider{endpoint: runtime.server.URL}
	orchestrator := sandbox.NewOrchestrator(db, provider, encKey, cfg)
	compileDeps := agentruntime.CompileDeps{
		DB:         db,
		EncKey:     encKey,
		SigningKey: []byte("upgrade-test-signing-key"),
		Cfg:        cfg,
	}
	return &agentSandboxUpgradeHarness{
		db:       db,
		encKey:   encKey,
		runtime:  runtime,
		provider: provider,
		handler:  NewAgentSandboxUpgradeHandler(db, orchestrator, compileDeps, &enqueue.MockClient{}),
	}
}

type agentSandboxUpgradeRuntime struct {
	server                  *httptest.Server
	mu                      sync.Mutex
	configCalls             int
	drainPOSTs              int
	drainGETs               int
	cancelPOSTs             int
	healthzCalls            int
	readyzCalls             int
	failDrainPOSTsRemaining int
}

func newAgentSandboxUpgradeRuntime(t *testing.T) *agentSandboxUpgradeRuntime {
	t.Helper()
	rt := &agentSandboxUpgradeRuntime{}
	rt.server = httptest.NewServer(http.HandlerFunc(rt.handle))
	t.Cleanup(rt.server.Close)
	return rt
}

func (rt *agentSandboxUpgradeRuntime) handle(w http.ResponseWriter, r *http.Request) {
	rt.mu.Lock()
	failDrainPOST := false
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/healthz":
		rt.healthzCalls++
	case r.Method == http.MethodGet && r.URL.Path == "/readyz":
		rt.readyzCalls++
	case r.Method == http.MethodPut && r.URL.Path == "/config":
		rt.configCalls++
	case r.Method == http.MethodPost && r.URL.Path == "/control/drain":
		rt.drainPOSTs++
		if rt.failDrainPOSTsRemaining > 0 {
			rt.failDrainPOSTsRemaining--
			failDrainPOST = true
		}
	case r.Method == http.MethodGet && r.URL.Path == "/control/drain":
		rt.drainGETs++
	case r.Method == http.MethodPost && r.URL.Path == "/control/drain/cancel":
		rt.cancelPOSTs++
	default:
		rt.mu.Unlock()
		http.NotFound(w, r)
		return
	}
	rt.mu.Unlock()

	if failDrainPOST {
		http.Error(w, "runtime unavailable", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	switch r.URL.Path {
	case "/config":
		_ = json.NewEncoder(w).Encode(map[string]any{"env_key_count": 1})
	case "/control/drain", "/control/drain/cancel":
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":                    "drained",
			"draining":                  true,
			"complete":                  true,
			"active_turns":              0,
			"pending_accepted_messages": 0,
			"pending_outbox_events":     0,
		})
	default:
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
	}
}

func (rt *agentSandboxUpgradeRuntime) failNextDrainPOSTs(n int) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.failDrainPOSTsRemaining = n
}

func (rt *agentSandboxUpgradeRuntime) assertDrainCalls(t *testing.T, wantPOSTs, wantGETs int) {
	t.Helper()
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.drainPOSTs != wantPOSTs || rt.drainGETs != wantGETs {
		t.Fatalf("drain calls post=%d get=%d want post=%d get=%d", rt.drainPOSTs, rt.drainGETs, wantPOSTs, wantGETs)
	}
}

type agentSandboxUpgradeProvider struct {
	endpoint string
	created  []sandbox.CreateSandboxOpts
	stopped  []string
	deleted  []string
}

func (p *agentSandboxUpgradeProvider) ID() string { return sandbox.ProviderMicrosandbox }

func (p *agentSandboxUpgradeProvider) Validate(context.Context) error { return nil }

func (p *agentSandboxUpgradeProvider) RuntimeLayout() sandbox.RuntimeLayout {
	return sandbox.RuntimeLayout{AgentRepoDir: "/workspace/repos", WorkspaceRepoDir: "/workspace/repos"}
}

func (p *agentSandboxUpgradeProvider) CreateSandbox(_ context.Context, opts sandbox.CreateSandboxOpts) (*sandbox.SandboxInfo, error) {
	p.created = append(p.created, opts)
	return &sandbox.SandboxInfo{ExternalID: fmt.Sprintf("upgrade-sandbox-%d", len(p.created)), Status: sandbox.StatusRunning}, nil
}

func (p *agentSandboxUpgradeProvider) StartSandbox(context.Context, string) error { return nil }

func (p *agentSandboxUpgradeProvider) StopSandbox(_ context.Context, externalID string) error {
	p.stopped = append(p.stopped, externalID)
	return nil
}

func (p *agentSandboxUpgradeProvider) ArchiveSandbox(context.Context, string) error { return nil }

func (p *agentSandboxUpgradeProvider) DeleteSandbox(_ context.Context, externalID string) error {
	p.deleted = append(p.deleted, externalID)
	return nil
}

func (p *agentSandboxUpgradeProvider) GetStatus(context.Context, string) (sandbox.SandboxStatus, error) {
	return sandbox.StatusRunning, nil
}

func (p *agentSandboxUpgradeProvider) GetEndpoint(context.Context, string, int) (string, error) {
	return p.endpoint, nil
}

func (p *agentSandboxUpgradeProvider) BuildTemplate(context.Context, sandbox.TemplateBuildRequest) (string, error) {
	return "", nil
}

func (p *agentSandboxUpgradeProvider) BuildTemplateWithLogs(context.Context, sandbox.TemplateBuildRequest, func(string)) (string, error) {
	return "", nil
}

func (p *agentSandboxUpgradeProvider) GetTemplateStatus(context.Context, string) (*sandbox.TemplateBuildStatus, error) {
	return &sandbox.TemplateBuildStatus{State: "ready"}, nil
}

func (p *agentSandboxUpgradeProvider) GetTemplateLogs(context.Context, string) (string, error) {
	return "", nil
}

func (p *agentSandboxUpgradeProvider) DeleteTemplate(context.Context, string) error { return nil }

func (p *agentSandboxUpgradeProvider) SetAutoStop(context.Context, string, int) error { return nil }

func (p *agentSandboxUpgradeProvider) SetAutoArchive(context.Context, string, int) error {
	return nil
}

func (p *agentSandboxUpgradeProvider) ExecuteCommand(context.Context, string, string) (string, error) {
	return "", nil
}

func (p *agentSandboxUpgradeProvider) ExecuteCommandWithTimeout(context.Context, string, string, time.Duration) (string, error) {
	return "", nil
}

func (p *agentSandboxUpgradeProvider) GetResourceUsage(context.Context, string) (*sandbox.ResourceUsage, error) {
	return &sandbox.ResourceUsage{}, nil
}
