package handler_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/apps"
	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sandbox"
)

// previewStubProvider covers the provider slice the preview-env endpoint
// exercises: ID (control rewrite) and GetEndpoint (docker branch).
type previewStubProvider struct {
	sandbox.Provider

	id string

	mu        sync.Mutex
	endpoints map[int]string
}

func (p *previewStubProvider) ID() string { return p.id }

func (p *previewStubProvider) GetEndpoint(_ context.Context, _ string, port int) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	endpoint, ok := p.endpoints[port]
	if !ok {
		return "", fmt.Errorf("stub provider: no endpoint for port %d", port)
	}
	return endpoint, nil
}

// previewEnvHarness wires the preview-env endpoint exactly as production
// mounts it, with a real org/agent/sandbox/app and the runtime-secret auth.
type previewEnvHarness struct {
	db     *gorm.DB
	router *chi.Mux
	encKey *crypto.SymmetricKey
	svc    *apps.Service

	org      model.Org
	agent    model.Agent
	sandbox  model.Sandbox
	channel  model.Channel
	sheet    model.Sheet
	app      *model.App
	secret   string // builder sandbox runtime secret (the bearer)
	provider *previewStubProvider
	authPEM  string
}

type previewHarnessOpts struct {
	providerID string // sandbox row + stub provider ID (default "microsandbox")
	runtimeURL string
	cfg        *config.Config
}

func newPreviewEnvHarness(t *testing.T, opts previewHarnessOpts) *previewEnvHarness {
	t.Helper()
	db := connectTestDB(t)

	if opts.providerID == "" {
		opts.providerID = sandbox.ProviderMicrosandbox
	}
	if opts.runtimeURL == "" {
		opts.runtimeURL = "https://7080-msb-builder-1.preview.test.dev"
	}
	if opts.cfg == nil {
		opts.cfg = &config.Config{FrontendURL: "https://web.test", APIWebhookBaseURL: "https://api.test"}
	}

	keyBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		t.Fatalf("generate enc key: %v", err)
	}
	encKey, err := crypto.NewSymmetricKey(base64.StdEncoding.EncodeToString(keyBytes))
	if err != nil {
		t.Fatalf("build enc key: %v", err)
	}

	h := &previewEnvHarness{
		db:       db,
		encKey:   encKey,
		provider: &previewStubProvider{id: opts.providerID, endpoints: map[int]string{}},
	}
	h.org = createTestOrg(t, db)
	h.agent = model.Agent{ID: uuid.New(), OrgID: &h.org.ID, TeamID: firstTeamID(t, db, h.org.ID), Name: "Preview Agent " + uuid.NewString(), Model: "test", Status: "active"}
	h.channel = model.Channel{ID: uuid.New(), OrgID: h.org.ID, Name: "preview-ch-" + uuid.NewString(), TeamID: h.agent.TeamID, DefaultAgentID: h.agent.ID}
	for _, seed := range []any{&h.agent, &h.channel} {
		if err := db.Create(seed).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	h.sheet = model.Sheet{ID: uuid.New(), OrgID: h.org.ID, ChannelID: h.channel.ID, Slug: "preview-" + uuid.NewString()[:8], Name: "Preview Sheet"}
	if err := db.Create(&h.sheet).Error; err != nil {
		t.Fatalf("seed sheet: %v", err)
	}

	h.secret = "runtime-secret-" + uuid.NewString()
	encrypted, err := encKey.EncryptString(h.secret)
	if err != nil {
		t.Fatalf("encrypt runtime secret: %v", err)
	}
	h.sandbox = model.Sandbox{
		OrgID:                  &h.org.ID,
		AgentID:                &h.agent.ID,
		ProviderID:             opts.providerID,
		ExternalID:             "msb-builder-1",
		RuntimeURL:             opts.runtimeURL,
		EncryptedRuntimeSecret: encrypted,
		Status:                 "running",
		ExposedPorts:           pq.Int64Array{3000, 8080},
	}
	if err := db.Create(&h.sandbox).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	t.Cleanup(func() {
		db.Delete(&model.App{}, "org_id = ?", h.org.ID)
		db.Delete(&model.Sandbox{}, "id = ?", h.sandbox.ID)
		db.Delete(&model.Sheet{}, "id = ?", h.sheet.ID)
		db.Delete(&model.Channel{}, "id = ?", h.channel.ID)
		db.Delete(&model.Agent{}, "id = ?", h.agent.ID)
	})

	h.authPEM = mustEncodePreviewAuthPEM(t)
	h.svc = apps.NewService(db, opts.cfg, encKey, h.provider, nil, nil, h.authPEM)

	app, err := h.svc.CreateApp(context.Background(), apps.CreateAppParams{
		OrgID:            h.org.ID,
		ChannelID:        h.channel.ID,
		SheetID:          h.sheet.ID,
		Name:             "Preview App " + uuid.NewString()[:8],
		CreatedByAgentID: &h.agent.ID,
	})
	if err != nil {
		t.Fatalf("create app: %v", err)
	}
	h.app = app

	uploadsHandler := handler.NewUploadsHandler(db, nil).
		WithStreamer(nil, encKey).
		WithAppsService(h.svc)
	router := chi.NewRouter()
	router.Get("/internal/agents/{agentID}/sandboxes/{sandboxID}/apps/{appID}/preview-env", uploadsHandler.AppPreviewEnv)
	h.router = router
	return h
}

func mustEncodePreviewAuthPEM(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	pem, err := apps.EncodeAuthPublicKeyPEM(&key.PublicKey)
	if err != nil {
		t.Fatalf("encode auth pem: %v", err)
	}
	return pem
}

// installAppsPlugin gives the harness agent the apps plugin the endpoint gates
// on (global plugin row + org install + team grant).
func (h *previewEnvHarness) installAppsPlugin(t *testing.T) {
	t.Helper()
	var plugin model.Plugin
	err := h.db.Where("slug = ? AND org_id IS NULL", apps.AppsPluginSlug).First(&plugin).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		plugin = model.Plugin{Slug: apps.AppsPluginSlug, Name: "Apps", Status: model.PluginStatusActive}
		if err := h.db.Create(&plugin).Error; err != nil {
			t.Fatalf("create apps plugin: %v", err)
		}
	} else if err != nil {
		t.Fatalf("load apps plugin: %v", err)
	} else if plugin.Status != model.PluginStatusActive {
		if err := h.db.Model(&plugin).Update("status", model.PluginStatusActive).Error; err != nil {
			t.Fatalf("activate apps plugin: %v", err)
		}
	}
	if err := h.db.Create(&model.OrgPluginInstall{ID: uuid.New(), OrgID: h.org.ID, PluginID: plugin.ID}).Error; err != nil {
		t.Fatalf("install apps plugin for org: %v", err)
	}
	grantPluginToAgentTeam(t, h.db, h.org.ID, h.agent.ID, plugin.ID)
	t.Cleanup(func() {
		h.db.Where("org_id = ? AND plugin_id = ?", h.org.ID, plugin.ID).Delete(&model.TeamPlugin{})
		h.db.Where("org_id = ? AND plugin_id = ?", h.org.ID, plugin.ID).Delete(&model.OrgPluginInstall{})
	})
}

func (h *previewEnvHarness) fetch(t *testing.T, bearer, appID, port string) *httptest.ResponseRecorder {
	t.Helper()
	path := "/internal/agents/" + h.agent.ID.String() + "/sandboxes/" + h.sandbox.ID.String() +
		"/apps/" + appID + "/preview-env"
	if port != "" {
		path += "?port=" + port
	}
	req := httptest.NewRequest("GET", path, nil)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}
