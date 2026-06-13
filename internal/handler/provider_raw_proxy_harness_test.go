package handler_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/nango"
)

type rawProviderProxyHarness struct {
	router            *chi.Mux
	agentID           uuid.UUID
	runtimeSecret     string
	providerConfigKey string
	nangoConnectionID string
}

func newRawProviderProxyHarness(t *testing.T, provider string, nangoHandler http.Handler) *rawProviderProxyHarness {
	t.Helper()

	db := connectTestDB(t)
	encKey := testSymmetricKey(t)
	nangoMock := httptest.NewServer(nangoHandler)
	t.Cleanup(nangoMock.Close)
	nangoClient := nango.NewClient(nangoMock.URL, "test-nango-secret")

	org := createTestOrg(t, db)
	user := createTestUser(t, db, fmt.Sprintf("%s-proxy-%s@example.com", provider, uuid.NewString()[:8]))
	integration := createTestIntegration(t, db, provider)
	agent := model.Agent{
		ID:     uuid.New(),
		OrgID:  &org.ID,
		Name:   provider + "-proxy-agent",
		Status: "active",
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}

	runtimeSecret := "test-runtime-secret-" + provider
	encryptedRuntimeSecret, err := encKey.EncryptString(runtimeSecret)
	if err != nil {
		t.Fatalf("encrypt runtime secret: %v", err)
	}
	sandbox := model.Sandbox{
		ID:                     uuid.New(),
		OrgID:                  &org.ID,
		AgentID:                &agent.ID,
		Status:                 "running",
		EncryptedRuntimeSecret: encryptedRuntimeSecret,
		RuntimeURL:             "http://localhost:7080",
	}
	if err := db.Create(&sandbox).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}

	connectionID := "nango-" + provider + "-conn"
	connection := model.Connection{
		ID:                uuid.New(),
		OrgID:             org.ID,
		UserID:            user.ID,
		IntegrationID:     integration.ID,
		NangoConnectionID: connectionID,
	}
	if err := db.Create(&connection).Error; err != nil {
		t.Fatalf("create connection: %v", err)
	}

	t.Cleanup(func() {
		db.Where("id = ?", connection.ID).Delete(&model.Connection{})
		db.Where("id = ?", sandbox.ID).Delete(&model.Sandbox{})
		db.Where("id = ?", agent.ID).Delete(&model.Agent{})
	})

	router := chi.NewRouter()
	switch provider {
	case "vercel":
		router.Handle("/internal/vercel-proxy/{agentID}/*", http.HandlerFunc(handler.NewVercelProxyHandler(db, encKey, nangoClient).Handle))
	case "glitchtip":
		router.Handle("/internal/glitchtip-proxy/{agentID}/*", http.HandlerFunc(handler.NewGlitchTipProxyHandler(db, encKey, nangoClient).Handle))
	case "slack":
		router.Handle("/internal/slack-proxy/{agentID}/*", http.HandlerFunc(handler.NewSlackProxyHandler(db, encKey, nangoClient).Handle))
	default:
		t.Fatalf("unsupported provider %q", provider)
	}

	return &rawProviderProxyHarness{
		router:            router,
		agentID:           agent.ID,
		runtimeSecret:     runtimeSecret,
		providerConfigKey: integration.UniqueKey,
		nangoConnectionID: connectionID,
	}
}
