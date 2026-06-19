package handler_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/auth"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/registry"
	"github.com/usehivy/hivy/internal/storage"
	"github.com/usehivy/hivy/internal/system"
)

type fakeImageGateway struct {
	text string
	err  error
	call system.ForwardCall
}

func (g *fakeImageGateway) Complete(ctx context.Context, call system.ForwardCall) (*system.CompletionResult, error) {
	g.call = call
	if g.err != nil {
		return nil, g.err
	}
	return &system.CompletionResult{
		Text: g.text,
		Usage: system.Usage{
			InputTokens:  321,
			OutputTokens: 123,
		},
		Model: call.Request.Model,
	}, nil
}

func (g *fakeImageGateway) Stream(context.Context, system.ForwardCall, http.ResponseWriter) (*system.CompletionResult, error) {
	return nil, errors.New("not implemented")
}

type imageDescribeHarness struct {
	db      *gorm.DB
	router  *chi.Mux
	gateway *fakeImageGateway
	org     *model.Org
	user    *model.User
	asset   *model.AgentAsset
}

type fakeImageAssetReader struct {
	data []byte
	err  error
}

func (r fakeImageAssetReader) Open(context.Context, string) (io.ReadCloser, error) {
	if r.err != nil {
		return nil, r.err
	}
	return io.NopCloser(bytes.NewReader(r.data)), nil
}

func newImageDescribeHarness(t *testing.T, opts ...func(*imageDescribeHarnessConfig)) *imageDescribeHarness {
	t.Helper()
	cfg := imageDescribeHarnessConfig{
		contentType: "image/png",
		modelText:   validImageAnalysisJSON(),
		seedCred:    true,
		registry:    registry.Global(),
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	db := connectTestDB(t)
	kms := newSystemTaskKMS(t)
	upstreamBase := "https://openrouter.test/api/v1"
	var cred *model.Credential
	if cfg.seedCred {
		cred = seedSystemCredential(t, db, kms, upstreamBase, "openrouter")
	}

	org := &model.Org{
		ID:        uuid.New(),
		Name:      "img-desc-" + uuid.NewString()[:8],
		RateLimit: 1000,
		Active:    true,
	}
	if err := db.Create(org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	user := &model.User{
		ID:               uuid.New(),
		Email:            "img-" + uuid.NewString()[:8] + "@test.local",
		Name:             "Image Tester",
		EmailConfirmedAt: tptr(time.Now()),
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	agent := &model.Agent{
		ID:     uuid.New(),
		OrgID:  &org.ID,
		Name:   "image-agent-" + uuid.NewString()[:8],
		Status: "active",
	}
	if err := db.Create(agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	sandbox := &model.Sandbox{
		ID:                     uuid.New(),
		OrgID:                  &org.ID,
		AgentID:                &agent.ID,
		Status:                 "running",
		ExternalID:             "external",
		RuntimeURL:             "http://runtime.local",
		EncryptedRuntimeSecret: []byte("encrypted"),
	}
	if err := db.Create(sandbox).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	asset := &model.AgentAsset{
		ID:          uuid.New(),
		OrgID:       org.ID,
		AgentID:     agent.ID,
		SandboxID:   sandbox.ID,
		Path:        "uploads",
		Filename:    "screenshot.png",
		Key:         "pub/e/" + agent.ID.String() + "/uploads/screenshot.png",
		PublicURL:   "https://assets.test/screenshot.png",
		ContentType: cfg.contentType,
		Bytes:       100,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := db.Create(asset).Error; err != nil {
		t.Fatalf("create asset: %v", err)
	}

	gateway := &fakeImageGateway{text: cfg.modelText, err: cfg.gatewayErr}
	h := handler.NewImageDescribeHandler(db, kms, cfg.registry, gateway, "https://api.usehivy.test")
	if cfg.assetReader != nil {
		h.WithAssetReader(cfg.assetReader)
	}

	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			req = middleware.WithOrg(req, org)
			req = middleware.WithUser(req, user)
			req = middleware.WithAuthClaims(req, &auth.AuthClaims{
				OrgID:  org.ID.String(),
				UserID: user.ID.String(),
			})
			next.ServeHTTP(w, req)
		})
	})
	r.Post("/v1/images/describe", h.Describe)

	out := &imageDescribeHarness{
		db:      db,
		router:  r,
		gateway: gateway,
		org:     org,
		user:    user,
		asset:   asset,
	}
	t.Cleanup(func() {
		if cred != nil {
			db.Where("id = ?", cred.ID).Delete(&model.Credential{})
		}
		db.Where("org_id = ?", org.ID).Delete(&model.Generation{})
		db.Where("id = ?", asset.ID).Delete(&model.AgentAsset{})
		db.Where("id = ?", sandbox.ID).Delete(&model.Sandbox{})
		db.Where("id = ?", agent.ID).Delete(&model.Agent{})
		db.Where("id = ?", org.ID).Delete(&model.Org{})
		db.Where("id = ?", user.ID).Delete(&model.User{})
	})
	return out
}

type imageDescribeHarnessConfig struct {
	contentType string
	modelText   string
	gatewayErr  error
	seedCred    bool
	registry    *registry.Registry
	assetReader storage.Reader
}

func withImageContentType(contentType string) func(*imageDescribeHarnessConfig) {
	return func(c *imageDescribeHarnessConfig) { c.contentType = contentType }
}

func withImageModelText(text string) func(*imageDescribeHarnessConfig) {
	return func(c *imageDescribeHarnessConfig) { c.modelText = text }
}

func withImageGatewayErr(err error) func(*imageDescribeHarnessConfig) {
	return func(c *imageDescribeHarnessConfig) { c.gatewayErr = err }
}

func withoutImageCredential() func(*imageDescribeHarnessConfig) {
	return func(c *imageDescribeHarnessConfig) { c.seedCred = false }
}

func withImageRegistry(reg *registry.Registry) func(*imageDescribeHarnessConfig) {
	return func(c *imageDescribeHarnessConfig) { c.registry = reg }
}

func withImageAssetReader(reader storage.Reader) func(*imageDescribeHarnessConfig) {
	return func(c *imageDescribeHarnessConfig) { c.assetReader = reader }
}

func (h *imageDescribeHarness) describe(t *testing.T, assetID uuid.UUID) *httptest.ResponseRecorder {
	t.Helper()
	body := `{"drive_asset_id":"` + assetID.String() + `","detail_level":"high"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/images/describe", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}
