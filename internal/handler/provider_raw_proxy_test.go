package handler_test

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/nango"
)

func TestVercelProxy_ForwardsRawRequestThroughNango(t *testing.T) {
	var capturedProviderConfigKey string
	var capturedConnectionID string
	var capturedPath string
	var capturedBody string

	nangoHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedProviderConfigKey = r.Header.Get("Provider-Config-Key")
		capturedConnectionID = r.Header.Get("Connection-Id")
		capturedPath = r.URL.RequestURI()
		body, _ := io.ReadAll(r.Body)
		capturedBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"deployments":[{"uid":"dpl_123","readyState":"READY"}]}`))
	})

	harness := newRawProviderProxyHarness(t, "vercel", nangoHandler)

	req := httptest.NewRequest(
		http.MethodPost,
		"/internal/vercel-proxy/"+harness.agentID.String()+"/v6/deployments?limit=1",
		bytes.NewReader([]byte(`{"name":"web"}`)),
	)
	req.Header.Set("Authorization", "Bearer "+harness.runtimeSecret)
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	harness.router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if capturedProviderConfigKey != harness.providerConfigKey {
		t.Fatalf("provider config key = %q, want %q", capturedProviderConfigKey, harness.providerConfigKey)
	}
	if capturedConnectionID != harness.nangoConnectionID {
		t.Fatalf("connection id = %q, want %q", capturedConnectionID, harness.nangoConnectionID)
	}
	if capturedPath != "/proxy/v6/deployments?limit=1" {
		t.Fatalf("path = %q, want proxy deployment path", capturedPath)
	}
	if capturedBody != `{"name":"web"}` {
		t.Fatalf("body = %q", capturedBody)
	}
}

func TestSlackProxy_RejectsNonSlackAPIPath(t *testing.T) {
	nangoHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("nango should not be called for invalid slack path")
	})
	harness := newRawProviderProxyHarness(t, "slack", nangoHandler)

	req := httptest.NewRequest(http.MethodGet, "/internal/slack-proxy/"+harness.agentID.String()+"/oauth/token", nil)
	req.Header.Set("Authorization", "Bearer "+harness.runtimeSecret)
	recorder := httptest.NewRecorder()
	harness.router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestVercelProxy_DeniesDeleteBeforeNango(t *testing.T) {
	nangoHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("nango should not be called for denied vercel delete")
	})
	harness := newRawProviderProxyHarness(t, "vercel", nangoHandler)

	req := httptest.NewRequest(
		http.MethodDelete,
		"/internal/vercel-proxy/"+harness.agentID.String()+"/v9/projects/prod-app",
		nil,
	)
	req.Header.Set("Authorization", "Bearer "+harness.runtimeSecret)
	recorder := httptest.NewRecorder()
	harness.router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "provider proxy safety policy") {
		t.Fatalf("expected safety policy message, got %s", recorder.Body.String())
	}
}

func TestSlackProxy_DeniesDestructiveMethodBeforeNango(t *testing.T) {
	nangoHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("nango should not be called for denied slack delete")
	})
	harness := newRawProviderProxyHarness(t, "slack", nangoHandler)

	req := httptest.NewRequest(
		http.MethodPost,
		"/internal/slack-proxy/"+harness.agentID.String()+"/api/chat.delete",
		bytes.NewReader([]byte(`{"channel":"C123","ts":"123.456"}`)),
	)
	req.Header.Set("Authorization", "Bearer "+harness.runtimeSecret)
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	harness.router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "performed by a human directly") {
		t.Fatalf("expected human dashboard guidance, got %s", recorder.Body.String())
	}
}

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
	agent := model.Employee{
		ID:     uuid.New(),
		OrgID:  &org.ID,
		Name:   provider + "-proxy-agent",
		Status: "active",
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create employee: %v", err)
	}

	runtimeSecret := "test-runtime-secret-" + provider
	encryptedRuntimeSecret, err := encKey.EncryptString(runtimeSecret)
	if err != nil {
		t.Fatalf("encrypt runtime secret: %v", err)
	}
	sandbox := model.Sandbox{
		ID:                     uuid.New(),
		OrgID:                  &org.ID,
		EmployeeID:             &agent.ID,
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
		db.Where("id = ?", agent.ID).Delete(&model.Employee{})
	})

	router := chi.NewRouter()
	switch provider {
	case "vercel":
		router.Handle("/internal/vercel-proxy/{employeeID}/*", http.HandlerFunc(handler.NewVercelProxyHandler(db, encKey, nangoClient).Handle))
	case "slack":
		router.Handle("/internal/slack-proxy/{employeeID}/*", http.HandlerFunc(handler.NewSlackProxyHandler(db, encKey, nangoClient).Handle))
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
