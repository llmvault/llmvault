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

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/credentials"
	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/employeeruntime"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/nango"
	"github.com/usehivy/hivy/internal/registry"
	"github.com/usehivy/hivy/internal/sandbox"
)

type stubEmployeeProvider struct {
	mu             sync.Mutex
	endpoint       string
	failOnCreate   bool
	createdCount   int
	deletedCount   int
	restartCount   int
	lastCreateOpts sandbox.CreateSandboxOpts
}

func (s *stubEmployeeProvider) ID() string { return sandbox.ProviderDaytona }

func (s *stubEmployeeProvider) Validate(context.Context) error { return nil }

func (s *stubEmployeeProvider) RuntimeLayout() sandbox.RuntimeLayout {
	return sandbox.RuntimeLayout{
		AgentRepoDir:    "/home/daytona/repos",
		EmployeeRepoDir: "/workspace/repos",
	}
}

func (s *stubEmployeeProvider) CreateSandbox(_ context.Context, opts sandbox.CreateSandboxOpts) (*sandbox.SandboxInfo, error) {
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

func (s *stubEmployeeProvider) GetEndpoint(_ context.Context, _ string, _ int) (string, error) {
	return s.endpoint, nil
}

func (s *stubEmployeeProvider) DeleteSandbox(_ context.Context, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deletedCount++
	return nil
}

func (s *stubEmployeeProvider) StartSandbox(context.Context, string) error { return nil }
func (s *stubEmployeeProvider) StopSandbox(context.Context, string) error  { return nil }
func (s *stubEmployeeProvider) RestartSandbox(context.Context, string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.restartCount++
	return nil
}
func (s *stubEmployeeProvider) ArchiveSandbox(context.Context, string) error { return nil }
func (s *stubEmployeeProvider) GetStatus(context.Context, string) (sandbox.SandboxStatus, error) {
	return sandbox.StatusRunning, nil
}
func (s *stubEmployeeProvider) BuildTemplate(context.Context, sandbox.TemplateBuildRequest) (string, error) {
	return "", nil
}
func (s *stubEmployeeProvider) BuildTemplateWithLogs(context.Context, sandbox.TemplateBuildRequest, func(string)) (string, error) {
	return "", nil
}
func (s *stubEmployeeProvider) GetTemplateStatus(context.Context, string) (*sandbox.TemplateBuildStatus, error) {
	return &sandbox.TemplateBuildStatus{State: "ready"}, nil
}
func (s *stubEmployeeProvider) GetTemplateLogs(context.Context, string) (string, error) {
	return "", nil
}
func (s *stubEmployeeProvider) DeleteTemplate(context.Context, string) error      { return nil }
func (s *stubEmployeeProvider) SetAutoStop(context.Context, string, int) error    { return nil }
func (s *stubEmployeeProvider) SetAutoArchive(context.Context, string, int) error { return nil }
func (s *stubEmployeeProvider) ExecuteCommand(context.Context, string, string) (string, error) {
	return "", nil
}
func (s *stubEmployeeProvider) ExecuteCommandWithTimeout(ctx context.Context, externalID string, command string, _ time.Duration) (string, error) {
	return s.ExecuteCommand(ctx, externalID, command)
}
func (s *stubEmployeeProvider) GetResourceUsage(context.Context, string) (*sandbox.ResourceUsage, error) {
	return &sandbox.ResourceUsage{}, nil
}

type employeeHarness struct {
	db         *gorm.DB
	handler    *handler.EmployeeHandler
	router     *chi.Mux
	provider   *stubEmployeeProvider
	enqueuer   *enqueue.MockClient
	encKey     *crypto.SymmetricKey
	kms        *crypto.KeyWrapper
	cfg        *config.Config
	sidecar    *sidecarStub
	sidecarSrv *httptest.Server
}

func newEmployeeHarness(t *testing.T) *employeeHarness {
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
	sidecarSrv := newEmployeeSidecarServer(t, stub)
	t.Cleanup(sidecarSrv.Close)

	provider := &stubEmployeeProvider{endpoint: sidecarSrv.URL}
	encKey := newTestEncKey(t)
	kms := newTestKMS(t)

	cfg := &config.Config{
		SandboxesRuntimeBaseImage:       "ghcr.io/usehivy/hivy-sandboxes-runtime:test",
		SandboxesRuntimeSpecialistImage: "ghcr.io/usehivy/hivy-sandboxes-runtime-specialist:test",
		SpecialistSandboxHost:           "cp.hivy.test",
		ProxyHost:                       "proxy.hivy.test",
		MCPBaseURL:                      "https://mcp.hivy.test",
	}
	orch := sandbox.NewOrchestrator(db, provider, encKey, cfg)
	nangoSrv := httptest.NewServer(newNangoConnMock(&nangoConnMockConfig{}))
	t.Cleanup(nangoSrv.Close)

	compileDeps := employeeruntime.CompileDeps{
		DB:         db,
		Picker:     credentials.NewPickerWithRegistry(db, registry.Global()),
		KMS:        kms,
		EncKey:     encKey,
		Nango:      nango.NewClient(nangoSrv.URL, "test-secret-key"),
		SigningKey: []byte("test-signing-key-32-bytes-long!!"),
		Cfg:        cfg,
	}
	enq := &enqueue.MockClient{}
	h := handler.NewEmployeeHandler(db, orch, compileDeps, registry.Global())
	h.SetEnqueuer(enq)

	r := chi.NewRouter()
	r.Route("/v1/employees", func(r chi.Router) {
		r.Use(middleware.ResolveOrgFromHeader(db))
		r.Get("/", h.List)
		r.Get("/{id}", h.Get)
		r.Get("/{id}/sessions", h.ListSessions)
		r.Get("/{id}/sessions/{sessionID}/events", h.ListSessionEvents)
		r.Post("/{id}/sessions/messages", h.SendSessionMessage)
		r.Get("/{id}/sessions/{sessionID}/streams/{streamID}", h.StreamSession)
		r.Get("/{id}/specialists", h.ListSpecialists)
		r.Patch("/{id}/specialists/{slug}", h.UpdateSpecialist)
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireOrgAdmin(db))
			r.Patch("/{id}/model", h.UpdateModel)
			r.Post("/{id}/sync", h.Sync)
			r.Post("/{id}/sandbox/reboot", h.RebootSandbox)
			r.Post("/{id}/sandbox/upgrade", h.StartSandboxUpgrade)
			r.Get("/{id}/sandbox/upgrades/{upgradeID}", h.GetSandboxUpgrade)
		})
	})

	return &employeeHarness{
		db: db, handler: h, router: r, provider: provider, enqueuer: enq,
		encKey: encKey, kms: kms, cfg: cfg,
		sidecar: stub, sidecarSrv: sidecarSrv,
	}
}
