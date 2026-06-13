package handler_test

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/credentials"
	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/nango"
	"github.com/usehivy/hivy/internal/registry"
	"github.com/usehivy/hivy/internal/sandbox"
)

type stubAgentProvider struct {
	mu             sync.Mutex
	endpoint       string
	failOnCreate   bool
	createdCount   int
	deletedCount   int
	restartCount   int
	lastCreateOpts sandbox.CreateSandboxOpts
}

func (s *stubAgentProvider) ID() string { return sandbox.ProviderDaytona }

func (s *stubAgentProvider) Validate(context.Context) error { return nil }

func (s *stubAgentProvider) RuntimeLayout() sandbox.RuntimeLayout {
	return sandbox.RuntimeLayout{
		AgentRepoDir:     "/home/daytona/repos",
		WorkspaceRepoDir: "/workspace/repos",
	}
}

func (s *stubAgentProvider) CreateSandbox(_ context.Context, opts sandbox.CreateSandboxOpts) (*sandbox.SandboxInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failOnCreate {
		return nil, errors.New("stub: provider create failed")
	}
	s.createdCount++
	s.lastCreateOpts = opts
	return &sandbox.SandboxInfo{
		ExternalID: fmt.Sprintf("stub-sb-%d", s.createdCount),
		Status:     sandbox.StatusRunning,
	}, nil
}

func (s *stubAgentProvider) GetEndpoint(_ context.Context, _ string, _ int) (string, error) {
	return s.endpoint, nil
}

func (s *stubAgentProvider) DeleteSandbox(_ context.Context, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deletedCount++
	return nil
}

func (s *stubAgentProvider) StartSandbox(context.Context, string) error { return nil }
func (s *stubAgentProvider) StopSandbox(context.Context, string) error  { return nil }
func (s *stubAgentProvider) RestartSandbox(context.Context, string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.restartCount++
	return nil
}
func (s *stubAgentProvider) ArchiveSandbox(context.Context, string) error { return nil }
func (s *stubAgentProvider) GetStatus(context.Context, string) (sandbox.SandboxStatus, error) {
	return sandbox.StatusRunning, nil
}
func (s *stubAgentProvider) BuildTemplate(context.Context, sandbox.TemplateBuildRequest) (string, error) {
	return "", nil
}
func (s *stubAgentProvider) BuildTemplateWithLogs(context.Context, sandbox.TemplateBuildRequest, func(string)) (string, error) {
	return "", nil
}
func (s *stubAgentProvider) GetTemplateStatus(context.Context, string) (*sandbox.TemplateBuildStatus, error) {
	return &sandbox.TemplateBuildStatus{State: "ready"}, nil
}
func (s *stubAgentProvider) GetTemplateLogs(context.Context, string) (string, error) {
	return "", nil
}
func (s *stubAgentProvider) DeleteTemplate(context.Context, string) error      { return nil }
func (s *stubAgentProvider) SetAutoStop(context.Context, string, int) error    { return nil }
func (s *stubAgentProvider) SetAutoArchive(context.Context, string, int) error { return nil }
func (s *stubAgentProvider) ExecuteCommand(context.Context, string, string) (string, error) {
	return "", nil
}
func (s *stubAgentProvider) ExecuteCommandWithTimeout(ctx context.Context, externalID string, command string, _ time.Duration) (string, error) {
	return s.ExecuteCommand(ctx, externalID, command)
}
func (s *stubAgentProvider) GetResourceUsage(context.Context, string) (*sandbox.ResourceUsage, error) {
	return &sandbox.ResourceUsage{}, nil
}

type agentHarness struct {
	db         *gorm.DB
	handler    *handler.AgentHandler
	router     *chi.Mux
	provider   *stubAgentProvider
	enqueuer   *enqueue.MockClient
	encKey     *crypto.SymmetricKey
	kms        *crypto.KeyWrapper
	cfg        *config.Config
	sidecar    *sidecarStub
	sidecarSrv *httptest.Server
}

func newAgentHarness(t *testing.T) *agentHarness {
	t.Helper()
	db := connectTestDB(t)
	defaultSkillNames := []string{
		"git-github",
		"drive",
		"agent-browser",
	}
	db.Unscoped().
		Where("org_id IS NULL AND (name IN ? OR slug IN ?)", defaultSkillNames, defaultSkillNames).
		Delete(&model.Skill{})

	stub := &sidecarStub{}
	sidecarSrv := newAgentSidecarServer(t, stub)
	t.Cleanup(sidecarSrv.Close)

	provider := &stubAgentProvider{endpoint: sidecarSrv.URL}
	encKey := newTestEncKey(t)
	kms := newTestKMS(t)

	cfg := &config.Config{
		SandboxesRuntimeBaseImage: "ghcr.io/usehivy/hivy-sandboxes-runtime:test",
		APIWebhookBaseURL:         "https://cp.hivy.test",
		ProxyHost:                 "proxy.hivy.test",
		MCPBaseURL:                "https://mcp.hivy.test",
	}
	orch := sandbox.NewOrchestrator(db, provider, encKey, cfg)
	nangoSrv := httptest.NewServer(newNangoConnMock(&nangoConnMockConfig{}))
	t.Cleanup(nangoSrv.Close)

	compileDeps := agentruntime.CompileDeps{
		DB:         db,
		Picker:     credentials.NewPickerWithRegistry(db, registry.Global()),
		KMS:        kms,
		EncKey:     encKey,
		Nango:      nango.NewClient(nangoSrv.URL, "test-secret-key"),
		SigningKey: []byte("test-signing-key-32-bytes-long!!"),
		Cfg:        cfg,
	}
	enq := &enqueue.MockClient{}
	h := handler.NewAgentHandler(db, orch, compileDeps, registry.Global())
	h.SetEnqueuer(enq)

	r := chi.NewRouter()
	r.Route("/v1/agents", func(r chi.Router) {
		r.Use(middleware.ResolveOrgFromHeader(db))
		r.Get("/", h.List)
		r.Get("/{id}", h.Get)
		r.Get("/{id}/sessions", h.ListSessions)
		r.Get("/{id}/sessions/{sessionID}/events", h.ListSessionEvents)
		r.Post("/{id}/sessions/messages", h.SendSessionMessage)
		r.Get("/{id}/sessions/{sessionID}/streams/{streamID}", h.StreamSession)
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireOrgAdmin(db))
			r.Post("/", h.Create)
			r.Patch("/{id}", h.Update)
			r.Delete("/{id}", h.Archive)
			r.Patch("/{id}/model", h.UpdateModel)
			r.Post("/{id}/sync", h.Sync)
			r.Post("/{id}/sandbox/reboot", h.RebootSandbox)
			r.Post("/{id}/sandbox/upgrade", h.StartSandboxUpgrade)
			r.Get("/{id}/sandbox/upgrades/{upgradeID}", h.GetSandboxUpgrade)
		})
	})

	return &agentHarness{
		db: db, handler: h, router: r, provider: provider, enqueuer: enq,
		encKey: encKey, kms: kms, cfg: cfg,
		sidecar: stub, sidecarSrv: sidecarSrv,
	}
}
