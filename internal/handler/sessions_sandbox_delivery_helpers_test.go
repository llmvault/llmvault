package handler_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/precontext"
	sandboxpkg "github.com/usehivy/hivy/internal/sandbox"
)

func newSessionRuntimeHarness(t *testing.T, runtime *sessionRuntimeStub, createErr error, builders ...precontext.Builder) (*sessionHarness, *sessionSandboxProviderStub) {
	t.Helper()
	db := connectTestDB(t)
	enq := &enqueue.MockClient{}
	encKey := sessionTestEncKey(t)
	cfg := &config.Config{
		SandboxProviderID:        sandboxpkg.ProviderMicrosandbox,
		SandboxesRuntimeImageTag: "delivery-test-amd64",
		APIWebhookBaseURL:        "https://api.example.test",
		ProxyHost:                "https://proxy.example.test",
		MCPBaseURL:               "https://mcp.example.test",
	}
	provider := &sessionSandboxProviderStub{endpoint: runtime.server.URL, createErr: createErr}
	orchestrator := sandboxpkg.NewOrchestrator(db, provider, encKey, cfg)
	compileDeps := agentruntime.CompileDeps{
		DB:         db,
		EncKey:     encKey,
		SigningKey: []byte("session-delivery-signing-key"),
		Cfg:        cfg,
	}
	orchestrator.SetAgentRuntimeConfigPusher(func(ctx context.Context, sb *model.Sandbox, push sandboxpkg.AgentRuntimeConfigPush) error {
		if push.Agent != nil {
			return agentruntime.PushAgentRuntimeConfigWithProxyTokenOptions(ctx, compileDeps, push.Agent, sb, push.ProxyToken, push.RuntimeOptions)
		}
		return agentruntime.PushAgentRuntimeConfigForSandboxWithProxyTokenOptions(ctx, compileDeps, sb, push.ProxyToken, push.RuntimeOptions)
	})
	h := handler.NewSessionHandler(db, enq).
		WithRuntimeStreamKey(encKey).
		WithRuntimeDelivery(orchestrator, compileDeps)
	if len(builders) > 0 {
		h.WithPreContextBuilder(builders[0])
	}
	r := chi.NewRouter()
	r.Route("/v1", func(r chi.Router) {
		r.Use(middleware.ResolveOrgFromHeader(db))
		r.Use(middleware.RequireAPIKeyScopeOrJWT("sessions"))
		r.Get("/channels/{id}/sessions", h.ListChannelSessions)
		r.Get("/sessions", h.List)
		r.Post("/sessions", h.Create)
		r.Get("/sessions/{id}", h.Get)
		r.Patch("/sessions/{id}", h.Update)
		r.Post("/sessions/{id}/messages", h.SendMessage)
		r.Post("/sessions/{id}/input-responses", h.RespondToInput)
		r.Post("/sessions/{id}/interrupt", h.Interrupt)
		r.Get("/sessions/{id}/events", h.ListEvents)
		r.Post("/sessions/{id}/sandbox-access", h.SandboxAccess)
		r.Put("/sessions/{id}/participants/{userID}", h.PutParticipant)
		r.Delete("/sessions/{id}/participants/{userID}", h.DeleteParticipant)
	})
	return &sessionHarness{db: db, router: r, enqueuer: enq, orchestrator: orchestrator, compileDeps: compileDeps, runtime: runtime}, provider
}

type sessionSandboxProviderStub struct {
	endpoint  string
	createErr error
	created   []sandboxpkg.CreateSandboxOpts
	started   []string
	deleted   []string
	autoStops []int
}

func (p *sessionSandboxProviderStub) ID() string { return sandboxpkg.ProviderMicrosandbox }

func (p *sessionSandboxProviderStub) Validate(context.Context) error { return nil }

func (p *sessionSandboxProviderStub) RuntimeLayout() sandboxpkg.RuntimeLayout {
	return sandboxpkg.RuntimeLayout{AgentRepoDir: "/workspace/repos", WorkspaceRepoDir: "/workspace/repos"}
}

func (p *sessionSandboxProviderStub) CreateSandbox(_ context.Context, opts sandboxpkg.CreateSandboxOpts) (*sandboxpkg.SandboxInfo, error) {
	p.created = append(p.created, opts)
	if p.createErr != nil {
		return nil, p.createErr
	}
	return &sandboxpkg.SandboxInfo{ExternalID: fmt.Sprintf("delivery-sandbox-%d", len(p.created)), Status: sandboxpkg.StatusRunning}, nil
}

func (p *sessionSandboxProviderStub) StartSandbox(_ context.Context, externalID string) error {
	p.started = append(p.started, externalID)
	return nil
}

func (p *sessionSandboxProviderStub) StopSandbox(context.Context, string) error { return nil }

func (p *sessionSandboxProviderStub) ArchiveSandbox(context.Context, string) error { return nil }

func (p *sessionSandboxProviderStub) DeleteSandbox(_ context.Context, externalID string) error {
	p.deleted = append(p.deleted, externalID)
	return nil
}

func (p *sessionSandboxProviderStub) GetStatus(context.Context, string) (sandboxpkg.SandboxStatus, error) {
	return sandboxpkg.StatusRunning, nil
}

func (p *sessionSandboxProviderStub) GetEndpoint(context.Context, string, int) (string, error) {
	return p.endpoint, nil
}

func (p *sessionSandboxProviderStub) BuildTemplate(context.Context, sandboxpkg.TemplateBuildRequest) (string, error) {
	return "", nil
}

func (p *sessionSandboxProviderStub) BuildTemplateWithLogs(context.Context, sandboxpkg.TemplateBuildRequest, func(string)) (string, error) {
	return "", nil
}

func (p *sessionSandboxProviderStub) GetTemplateStatus(context.Context, string) (*sandboxpkg.TemplateBuildStatus, error) {
	return &sandboxpkg.TemplateBuildStatus{State: "ready"}, nil
}

func (p *sessionSandboxProviderStub) GetTemplateLogs(context.Context, string) (string, error) {
	return "", nil
}

func (p *sessionSandboxProviderStub) DeleteTemplate(context.Context, string) error { return nil }

func (p *sessionSandboxProviderStub) SetAutoStop(_ context.Context, _ string, intervalMinutes int) error {
	p.autoStops = append(p.autoStops, intervalMinutes)
	return nil
}

func (p *sessionSandboxProviderStub) SetAutoArchive(context.Context, string, int) error {
	return nil
}

func (p *sessionSandboxProviderStub) ExecuteCommand(context.Context, string, string) (string, error) {
	return "", nil
}

func (p *sessionSandboxProviderStub) ExecuteCommandWithTimeout(context.Context, string, string, time.Duration) (string, error) {
	return "", nil
}

func (p *sessionSandboxProviderStub) GetResourceUsage(context.Context, string) (*sandboxpkg.ResourceUsage, error) {
	return &sandboxpkg.ResourceUsage{}, nil
}

func markSessionAgentSession(t *testing.T, h *sessionHarness, fx *sessionFixture) {
	t.Helper()
}

func seedSessionRuntimeSandbox(t *testing.T, h *sessionHarness, fx sessionFixture, runtimeURL string, status string) model.Sandbox {
	t.Helper()
	encSecret, err := sessionTestEncKey(t).EncryptString("runtime-delivery-secret-" + uuid.NewString())
	if err != nil {
		t.Fatalf("encrypt runtime secret: %v", err)
	}
	sb := model.Sandbox{
		OrgID:                  &fx.org.ID,
		AgentID:                &fx.agent.ID,
		ProviderID:             sandboxpkg.ProviderMicrosandbox,
		ExternalID:             "delivery-existing-" + uuid.NewString()[:8],
		RuntimeURL:             runtimeURL,
		EncryptedRuntimeSecret: encSecret,
		Status:                 status,
	}
	if err := h.db.Create(&sb).Error; err != nil {
		t.Fatalf("create session sandbox runtime sandbox: %v", err)
	}
	return sb
}

func assertNoSessionCreateRows(t *testing.T, h *sessionHarness, orgID uuid.UUID) {
	t.Helper()
	for name, modelValue := range map[string]any{
		"sessions":              &model.Session{},
		"session_events":        &model.SessionEvent{},
		"session_message_queue": &model.SessionMessageQueue{},
		"sandboxes":             &model.Sandbox{},
	} {
		var count int64
		if err := h.db.Model(modelValue).Where("org_id = ?", orgID).Count(&count).Error; err != nil {
			t.Fatalf("count %s: %v", name, err)
		}
		if count != 0 {
			t.Fatalf("%s count=%d, want 0", name, count)
		}
	}
}

func countActiveSessionAgentProxyTokens(t *testing.T, h *sessionHarness, orgID, agentID uuid.UUID, sandboxID *string) int64 {
	t.Helper()
	var count int64
	q := h.db.Model(&model.Token{}).
		Where("org_id = ? AND meta ->> ? = ? AND meta ->> ? = ? AND meta ->> ? = ? AND revoked_at IS NULL",
			orgID,
			model.TokenMetaAgentID, agentID.String(),
			model.TokenMetaType, model.TokenTypeAgentProxy,
			model.TokenMetaHarness, model.TokenHarnessAgentSandbox)
	if sandboxID != nil {
		q = q.Where("meta ->> ? = ?", model.TokenMetaSandboxID, *sandboxID)
	}
	if err := q.Count(&count).Error; err != nil {
		t.Fatalf("count active agent proxy tokens: %v", err)
	}
	return count
}
