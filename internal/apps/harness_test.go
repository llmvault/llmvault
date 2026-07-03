package apps

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"sync"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sandbox"
	"github.com/usehivy/hivy/internal/storage"
	"github.com/usehivy/hivy/internal/testdb"
)

func connectTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := testdb.DatabaseURL("DATABASE_URL", "HIVY_DATABASE_URL", "TEST_DATABASE_URL")
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("cannot connect to Postgres: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(3)
	sqlDB.SetMaxIdleConns(1)
	testdb.ApplyMigrations(t, db)
	t.Cleanup(func() { sqlDB.Close() })
	return db
}

// stubProvider implements the pieces of sandbox.Provider the apps service
// touches; every other method panics through the embedded nil interface.
type stubProvider struct {
	sandbox.Provider

	mu        sync.Mutex
	created   []sandbox.CreateSandboxOpts
	stopped   []string
	deleted   []string
	createErr error
	// endpoints maps container port → endpoint URL.
	endpoints map[int]string
	nextID    int
}

func (p *stubProvider) ID() string { return "stub" }

func (p *stubProvider) CreateSandbox(_ context.Context, opts sandbox.CreateSandboxOpts) (*sandbox.SandboxInfo, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.createErr != nil {
		return nil, p.createErr
	}
	p.created = append(p.created, opts)
	p.nextID++
	return &sandbox.SandboxInfo{ExternalID: fmt.Sprintf("stub-ext-%d", p.nextID), Status: sandbox.StatusRunning}, nil
}

func (p *stubProvider) GetEndpoint(_ context.Context, _ string, port int) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	endpoint, ok := p.endpoints[port]
	if !ok {
		return "", fmt.Errorf("stub provider: no endpoint for port %d", port)
	}
	return endpoint, nil
}

func (p *stubProvider) StopSandbox(_ context.Context, externalID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stopped = append(p.stopped, externalID)
	return nil
}

func (p *stubProvider) DeleteSandbox(_ context.Context, externalID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.deleted = append(p.deleted, externalID)
	return nil
}

// fakeStore is an in-memory ObjectStore.
type fakeStore struct {
	mu      sync.Mutex
	objects map[string][]byte
	deleted []string
}

func newFakeStore() *fakeStore {
	return &fakeStore{objects: map[string][]byte{}}
}

func (s *fakeStore) put(key string, data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.objects[key] = data
}

func (s *fakeStore) get(key string) ([]byte, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, ok := s.objects[key]
	return data, ok
}

func (s *fakeStore) Open(_ context.Context, key string) (io.ReadCloser, error) {
	data, ok := s.get(key)
	if !ok {
		return nil, fmt.Errorf("fake store: no object %q", key)
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (s *fakeStore) Stream(_ context.Context, key, _ string, body io.Reader) (*storage.StoredAsset, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}
	s.put(key, data)
	return &storage.StoredAsset{Key: key, Bytes: int64(len(data))}, nil
}

func (s *fakeStore) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.objects, key)
	s.deleted = append(s.deleted, key)
	return nil
}

func (s *fakeStore) PresignGet(_ context.Context, key string) (string, error) {
	return "https://s3.test/presigned/" + key, nil
}

// stubAuthorizer records the admitted keys and optionally rejects.
type stubAuthorizer struct {
	mu     sync.Mutex
	keys   []string
	reject error
}

func (a *stubAuthorizer) AuthorizeObjectKeys(_ context.Context, _ uuid.UUID, keys []string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.reject != nil {
		return a.reject
	}
	a.keys = append(a.keys, keys...)
	return nil
}

// appsTestHarness bundles the service and its seeded org/channel/sheet.
type appsTestHarness struct {
	db       *gorm.DB
	svc      *Service
	encKey   *crypto.SymmetricKey
	provider *stubProvider
	store    *fakeStore
	keys     *stubAuthorizer
	cfg      *config.Config

	org        model.Org
	agent      model.Agent
	channel    model.Channel
	sheet      model.Sheet
	otherSheet model.Sheet // second channel in the same org
	otherChan  model.Channel
}

func newAppsTestHarness(t *testing.T) *appsTestHarness {
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

	h := &appsTestHarness{
		db:       db,
		encKey:   encKey,
		provider: &stubProvider{endpoints: map[int]string{}},
		store:    newFakeStore(),
		keys:     &stubAuthorizer{},
		cfg: &config.Config{
			FrontendURL:       "https://web.test",
			APIWebhookBaseURL: "https://api.test",
		},
	}

	h.org = model.Org{ID: uuid.New(), Name: "apps-svc-" + uuid.NewString()[:8], RateLimit: 1000, Active: true}
	if err := db.Create(&h.org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	h.agent = model.Agent{ID: uuid.New(), OrgID: &h.org.ID, Name: "Apps Agent " + uuid.NewString(), Model: "test", Status: "active"}
	h.channel = model.Channel{ID: uuid.New(), OrgID: h.org.ID, Name: "apps-ch-" + uuid.NewString(), DefaultAgentID: h.agent.ID}
	h.otherChan = model.Channel{ID: uuid.New(), OrgID: h.org.ID, Name: "apps-other-" + uuid.NewString(), DefaultAgentID: h.agent.ID}
	for _, seed := range []any{&h.agent, &h.channel, &h.otherChan} {
		if err := db.Create(seed).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	h.sheet = model.Sheet{ID: uuid.New(), OrgID: h.org.ID, ChannelID: h.channel.ID, Slug: "tasks-" + uuid.NewString()[:8], Name: "Tasks"}
	h.otherSheet = model.Sheet{ID: uuid.New(), OrgID: h.org.ID, ChannelID: h.otherChan.ID, Slug: "other-" + uuid.NewString()[:8], Name: "Other"}
	for _, seed := range []any{&h.sheet, &h.otherSheet} {
		if err := db.Create(seed).Error; err != nil {
			t.Fatalf("seed sheet: %v", err)
		}
	}
	t.Cleanup(func() {
		db.Delete(&model.App{}, "org_id = ?", h.org.ID)
		db.Delete(&model.Sandbox{}, "org_id = ?", h.org.ID)
		db.Delete(&model.Org{}, "id = ?", h.org.ID)
	})

	authPEM, err := EncodeAuthPublicKeyPEM(&testRSAKey(t).PublicKey)
	if err != nil {
		t.Fatalf("encode auth public key: %v", err)
	}
	h.svc = NewService(db, h.cfg, encKey, h.provider, h.store, h.keys, authPEM)
	return h
}

func (h *appsTestHarness) createApp(t *testing.T, name string) *model.App {
	t.Helper()
	app, err := h.svc.CreateApp(context.Background(), CreateAppParams{
		OrgID:     h.org.ID,
		ChannelID: h.channel.ID,
		SheetID:   h.sheet.ID,
		Name:      name,
	})
	if err != nil {
		t.Fatalf("create app: %v", err)
	}
	return app
}

// seedDriveObjects stores source/bundle zips under the agent's drive prefix
// and returns their keys plus shas.
func (h *appsTestHarness) seedDriveObjects(t *testing.T, source, bundle []byte) (sourceKey, bundleKey, sourceSHA, bundleSHA string) {
	t.Helper()
	sourceKey = fmt.Sprintf("pub/e/%s/build/source.zip", h.agent.ID)
	bundleKey = fmt.Sprintf("pub/e/%s/build/bundle.zip", h.agent.ID)
	h.store.put(sourceKey, source)
	h.store.put(bundleKey, bundle)
	return sourceKey, bundleKey, sha256Hex(source), sha256Hex(bundle)
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
