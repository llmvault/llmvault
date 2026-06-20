package handler_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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
	sandboxpkg "github.com/usehivy/hivy/internal/sandbox"
)

func newSessionRuntimeHarness(t *testing.T, runtime *sessionSyncRuntime, createErr error) (*sessionHarness, *sessionSyncSandboxProvider) {
	t.Helper()
	db := connectTestDB(t)
	enq := &enqueue.MockClient{}
	encKey := sessionTestEncKey(t)
	cfg := &config.Config{
		SandboxProviderID:        sandboxpkg.ProviderMicrosandbox,
		SandboxesRuntimeImageTag: "sync-test-amd64",
		APIWebhookBaseURL:        "https://api.example.test",
		ProxyHost:                "https://proxy.example.test",
		MCPBaseURL:               "https://mcp.example.test",
	}
	provider := &sessionSyncSandboxProvider{endpoint: runtime.server.URL, createErr: createErr}
	orchestrator := sandboxpkg.NewOrchestrator(db, provider, encKey, cfg)
	compileDeps := agentruntime.CompileDeps{
		DB:         db,
		EncKey:     encKey,
		SigningKey: []byte("session-sync-signing-key"),
		Cfg:        cfg,
	}
	orchestrator.SetAgentRuntimeConfigPusher(func(ctx context.Context, sb *model.Sandbox) error {
		return agentruntime.PushAgentRuntimeConfigForSandbox(ctx, compileDeps, sb)
	})
	h := handler.NewSessionHandler(db, enq).
		WithRuntimeStreamKey(encKey).
		WithRuntimeDelivery(orchestrator, compileDeps)
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
		r.Post("/sessions/{id}/sandbox/wake", h.WakeSandbox)
		r.Post("/sessions/{id}/sandbox-access", h.SandboxAccess)
		r.Put("/sessions/{id}/participants/{userID}", h.PutParticipant)
		r.Delete("/sessions/{id}/participants/{userID}", h.DeleteParticipant)
	})
	return &sessionHarness{db: db, router: r, enqueuer: enq}, provider
}

type sessionSyncRuntime struct {
	server          *httptest.Server
	messageStatus   int
	configCalls     int
	messageCalls    int
	lastSessionID   string
	lastMessageText string
}

func newSessionSyncRuntime(t *testing.T, messageStatus int) *sessionSyncRuntime {
	t.Helper()
	runtime := &sessionSyncRuntime{messageStatus: messageStatus}
	runtime.server = httptest.NewServer(http.HandlerFunc(runtime.handle))
	t.Cleanup(runtime.server.Close)
	return runtime
}

func (rt *sessionSyncRuntime) handle(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/healthz":
		w.WriteHeader(http.StatusOK)
	case r.Method == http.MethodGet && r.URL.Path == "/readyz":
		w.WriteHeader(http.StatusOK)
	case r.Method == http.MethodPut && r.URL.Path == "/config":
		rt.configCalls++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"env_key_count": 1})
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/sessions/") && strings.HasSuffix(r.URL.Path, "/messages"):
		rt.handleMessage(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (rt *sessionSyncRuntime) handleMessage(w http.ResponseWriter, r *http.Request) {
	rt.messageCalls++
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) >= 3 {
		rt.lastSessionID = parts[1]
	}
	var body struct {
		Text string `json:"text"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	rt.lastMessageText = body.Text
	if rt.messageStatus >= http.StatusBadRequest {
		http.Error(w, "runtime rejected message", rt.messageStatus)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"session_id": rt.lastSessionID,
		"stream_id":  "stream-" + shortTestID(rt.lastSessionID),
		"stream_url": "/sessions/" + rt.lastSessionID + "/stream",
		"trace_id":   "trace-" + shortTestID(rt.lastSessionID),
		"turn_id":    "turn-" + shortTestID(rt.lastSessionID),
	})
}

type sessionSyncSandboxProvider struct {
	endpoint  string
	createErr error
	created   []sandboxpkg.CreateSandboxOpts
	started   []string
	deleted   []string
}

func (p *sessionSyncSandboxProvider) ID() string { return sandboxpkg.ProviderMicrosandbox }

func (p *sessionSyncSandboxProvider) Validate(context.Context) error { return nil }

func (p *sessionSyncSandboxProvider) RuntimeLayout() sandboxpkg.RuntimeLayout {
	return sandboxpkg.RuntimeLayout{AgentRepoDir: "/workspace/repos", WorkspaceRepoDir: "/workspace/repos"}
}

func (p *sessionSyncSandboxProvider) CreateSandbox(_ context.Context, opts sandboxpkg.CreateSandboxOpts) (*sandboxpkg.SandboxInfo, error) {
	p.created = append(p.created, opts)
	if p.createErr != nil {
		return nil, p.createErr
	}
	return &sandboxpkg.SandboxInfo{ExternalID: fmt.Sprintf("sync-sandbox-%d", len(p.created)), Status: sandboxpkg.StatusRunning}, nil
}

func (p *sessionSyncSandboxProvider) StartSandbox(_ context.Context, externalID string) error {
	p.started = append(p.started, externalID)
	return nil
}

func (p *sessionSyncSandboxProvider) StopSandbox(context.Context, string) error { return nil }

func (p *sessionSyncSandboxProvider) ArchiveSandbox(context.Context, string) error { return nil }

func (p *sessionSyncSandboxProvider) DeleteSandbox(_ context.Context, externalID string) error {
	p.deleted = append(p.deleted, externalID)
	return nil
}

func (p *sessionSyncSandboxProvider) GetStatus(context.Context, string) (sandboxpkg.SandboxStatus, error) {
	return sandboxpkg.StatusRunning, nil
}

func (p *sessionSyncSandboxProvider) GetEndpoint(context.Context, string, int) (string, error) {
	return p.endpoint, nil
}

func (p *sessionSyncSandboxProvider) BuildTemplate(context.Context, sandboxpkg.TemplateBuildRequest) (string, error) {
	return "", nil
}

func (p *sessionSyncSandboxProvider) BuildTemplateWithLogs(context.Context, sandboxpkg.TemplateBuildRequest, func(string)) (string, error) {
	return "", nil
}

func (p *sessionSyncSandboxProvider) GetTemplateStatus(context.Context, string) (*sandboxpkg.TemplateBuildStatus, error) {
	return &sandboxpkg.TemplateBuildStatus{State: "ready"}, nil
}

func (p *sessionSyncSandboxProvider) GetTemplateLogs(context.Context, string) (string, error) {
	return "", nil
}

func (p *sessionSyncSandboxProvider) DeleteTemplate(context.Context, string) error { return nil }

func (p *sessionSyncSandboxProvider) SetAutoStop(context.Context, string, int) error { return nil }

func (p *sessionSyncSandboxProvider) SetAutoArchive(context.Context, string, int) error {
	return nil
}

func (p *sessionSyncSandboxProvider) ExecuteCommand(context.Context, string, string) (string, error) {
	return "", nil
}

func (p *sessionSyncSandboxProvider) ExecuteCommandWithTimeout(context.Context, string, string, time.Duration) (string, error) {
	return "", nil
}

func (p *sessionSyncSandboxProvider) GetResourceUsage(context.Context, string) (*sandboxpkg.ResourceUsage, error) {
	return &sandboxpkg.ResourceUsage{}, nil
}

func markSessionAgentPerSession(t *testing.T, h *sessionHarness, fx *sessionFixture) {
	t.Helper()
	if err := h.db.Model(&model.Agent{}).Where("id = ?", fx.agent.ID).Update("sandbox_strategy", "per_session").Error; err != nil {
		t.Fatalf("mark agent per-session: %v", err)
	}
	fx.agent.SandboxStrategy = "per_session"
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

func shortTestID(value string) string {
	if len(value) < 8 {
		return value
	}
	return value[:8]
}
