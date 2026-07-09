package handler_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/apps"
	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sandbox"
	"github.com/usehivy/hivy/internal/storage"
)

// appsStubProvider implements the slice of sandbox.Provider the apps REST
// surface exercises; anything else panics via the embedded nil interface.
type appsStubProvider struct {
	sandbox.Provider

	mu        sync.Mutex
	endpoints map[int]string
	stopped   []string
}

func (p *appsStubProvider) ID() string { return "stub" }

func (p *appsStubProvider) GetEndpoint(_ context.Context, _ string, port int) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	endpoint, ok := p.endpoints[port]
	if !ok {
		return "", fmt.Errorf("stub provider: no endpoint for port %d", port)
	}
	return endpoint, nil
}

func (p *appsStubProvider) StopSandbox(_ context.Context, externalID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stopped = append(p.stopped, externalID)
	return nil
}

type appsFakeStore struct {
	mu      sync.Mutex
	objects map[string][]byte
}

func (s *appsFakeStore) Open(_ context.Context, key string) (io.ReadCloser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, ok := s.objects[key]
	if !ok {
		return nil, fmt.Errorf("no object %q", key)
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (s *appsFakeStore) Stream(_ context.Context, key, _ string, body io.Reader) (*storage.StoredAsset, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.objects == nil {
		s.objects = map[string][]byte{}
	}
	s.objects[key] = data
	return &storage.StoredAsset{Key: key, Bytes: int64(len(data))}, nil
}

func (s *appsFakeStore) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.objects, key)
	return nil
}

func (s *appsFakeStore) PresignGet(_ context.Context, key string) (string, error) {
	return "https://s3.test/" + key, nil
}

// get returns a stored object's bytes for assertions.
func (s *appsFakeStore) get(key string) ([]byte, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, ok := s.objects[key]
	return data, ok
}

type appsAllowAllKeys struct{}

func (appsAllowAllKeys) AuthorizeObjectKeys(context.Context, uuid.UUID, []string) error { return nil }

// appsRESTHarness drives the /v1/apps surface exactly as mounted in
// production (mountAppRoutes), with org+user injected like the sheets tests.
type appsRESTHarness struct {
	db       *gorm.DB
	svc      *apps.Service
	router   *chi.Mux
	provider *appsStubProvider
	store    *appsFakeStore
	encKey   *crypto.SymmetricKey
	rsaKey   *rsa.PrivateKey

	org       model.Org
	user      model.User
	agent     model.Agent
	channel   model.Channel
	otherChan model.Channel
	sheet     model.Sheet
	crossWire model.Sheet // sheet living in otherChan
}

func newAppsRESTHarness(t *testing.T) *appsRESTHarness {
	t.Helper()
	db := connectTestDB(t)

	keyBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		t.Fatalf("generate enc key: %v", err)
	}
	encKey, err := crypto.NewSymmetricKey(base64.StdEncoding.EncodeToString(keyBytes))
	if err != nil {
		t.Fatalf("build enc key: %v", err)
	}
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}

	h := &appsRESTHarness{
		db:       db,
		encKey:   encKey,
		rsaKey:   rsaKey,
		provider: &appsStubProvider{endpoints: map[int]string{}},
		store:    &appsFakeStore{},
	}
	h.org = createTestOrg(t, db)
	h.user = model.User{ID: uuid.New(), Email: "apps-rest-" + uuid.NewString() + "@example.com", Name: "Apps Rest User", AvatarURL: "https://cdn.test/a.png"}
	if err := db.Create(&h.user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	membership := model.OrgMembership{UserID: h.user.ID, OrgID: h.org.ID, Role: "member"}
	if err := db.Create(&membership).Error; err != nil {
		t.Fatalf("create membership: %v", err)
	}
	h.agent = model.Agent{ID: uuid.New(), OrgID: &h.org.ID, TeamID: firstTeamID(t, db, h.org.ID), Name: "Apps Rest Agent " + uuid.NewString(), Model: "test", Status: "active"}
	h.channel = model.Channel{ID: uuid.New(), OrgID: h.org.ID, Name: "apps-rest-" + uuid.NewString(), TeamID: h.agent.TeamID, DefaultAgentID: h.agent.ID}
	h.otherChan = model.Channel{ID: uuid.New(), OrgID: h.org.ID, Name: "apps-rest-other-" + uuid.NewString(), TeamID: h.agent.TeamID, DefaultAgentID: h.agent.ID}
	for _, seed := range []any{&h.agent, &h.channel, &h.otherChan} {
		if err := db.Create(seed).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	// The user is an active member of the channels' team so member-authenticated
	// launches resolve channel access (public channels open to their team).
	if err := db.Create(&model.TeamMember{OrgID: h.org.ID, TeamID: h.agent.TeamID, UserID: h.user.ID, Role: "member"}).Error; err != nil {
		t.Fatalf("seed team member: %v", err)
	}
	h.sheet = model.Sheet{ID: uuid.New(), OrgID: h.org.ID, ChannelID: h.channel.ID, Slug: "apps-rest-" + uuid.NewString()[:8], Name: "Rest Sheet"}
	h.crossWire = model.Sheet{ID: uuid.New(), OrgID: h.org.ID, ChannelID: h.otherChan.ID, Slug: "apps-cross-" + uuid.NewString()[:8], Name: "Cross Sheet"}
	for _, seed := range []any{&h.sheet, &h.crossWire} {
		if err := db.Create(seed).Error; err != nil {
			t.Fatalf("seed sheet: %v", err)
		}
	}
	t.Cleanup(func() {
		db.Delete(&model.App{}, "org_id = ?", h.org.ID)
		db.Delete(&model.Sandbox{}, "org_id = ?", h.org.ID)
		db.Delete(&model.Sheet{}, "org_id = ?", h.org.ID)
		db.Delete(&model.OrgMembership{}, "user_id = ?", h.user.ID)
		db.Delete(&model.User{}, "id = ?", h.user.ID)
	})

	authPEM, err := apps.EncodeAuthPublicKeyPEM(&rsaKey.PublicKey)
	if err != nil {
		t.Fatalf("encode auth pem: %v", err)
	}
	cfg := &config.Config{FrontendURL: "https://web.test", APIWebhookBaseURL: "https://api.test"}
	h.svc = apps.NewService(db, cfg, encKey, h.provider, h.store, appsAllowAllKeys{}, authPEM)

	appsHandler := handler.NewAppsHandler(db, h.svc, rsaKey)
	// Mirrors production registration (mountAppRoutes).
	router := chi.NewRouter()
	router.Route("/v1/apps", func(r chi.Router) {
		r.Post("/", appsHandler.Create)
		r.Get("/", appsHandler.List)
		r.Route("/{appID}", func(r chi.Router) {
			r.Get("/", appsHandler.Get)
			r.Delete("/", appsHandler.Archive)
			r.Get("/launch", appsHandler.Launch)
			r.Post("/versions", appsHandler.Versions)
		})
	})
	h.router = router
	return h
}

// do issues a request with org+user context (the /v1 auth + ResolveUser
// middlewares' output). withUser=false simulates an API-key caller carrying
// only org identity (API-key claims, no resolved user).
func (h *appsRESTHarness) do(t *testing.T, method, path string, body any, withUser bool) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &h.org)
	if withUser {
		req = middleware.WithUser(req, &h.user)
	} else {
		req = middleware.WithAPIKeyClaims(req, &middleware.APIKeyClaims{OrgID: h.org.ID.String()})
	}
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

// createAppViaAPI POSTs /v1/apps and returns the created app's ID.
func (h *appsRESTHarness) createAppViaAPI(t *testing.T, name string) string {
	t.Helper()
	rr := h.do(t, "POST", "/v1/apps", map[string]string{
		"channel_id": h.channel.ID.String(),
		"sheet_id":   h.sheet.ID.String(),
		"name":       name,
	}, true)
	if rr.Code != 201 {
		t.Fatalf("create app status = %d body=%s", rr.Code, rr.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	return created.ID
}

// markDeployed simulates a completed deploy: a sandbox row plus running
// status, so Launch can resolve the app's 8080 endpoint.
func (h *appsRESTHarness) markDeployed(t *testing.T, appID string) {
	t.Helper()
	encrypted, err := h.encKey.EncryptString("hvapp_test_secret")
	if err != nil {
		t.Fatalf("encrypt secret: %v", err)
	}
	sb := model.Sandbox{
		OrgID:                  &h.org.ID,
		ProviderID:             "stub",
		ExternalID:             "ext-launch",
		RuntimeURL:             "http://127.0.0.1:1",
		EncryptedRuntimeSecret: encrypted,
		Status:                 "running",
	}
	if err := h.db.Create(&sb).Error; err != nil {
		t.Fatalf("create sandbox row: %v", err)
	}
	if err := h.db.Model(&model.App{}).Where("id = ?", appID).Updates(map[string]any{
		"sandbox_id": sb.ID,
		"status":     model.AppStatusRunning,
	}).Error; err != nil {
		t.Fatalf("mark app deployed: %v", err)
	}
}
