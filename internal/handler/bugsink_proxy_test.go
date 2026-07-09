package handler_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/nango"
)

type bugsinkTestHarness struct {
	db            *gorm.DB
	router        *chi.Mux
	encKey        *crypto.SymmetricKey
	orgID         uuid.UUID
	agentID       uuid.UUID
	runtimeSecret string
}

func newBugsinkHarness(t *testing.T, nangoHandler http.Handler) *bugsinkTestHarness {
	t.Helper()

	db := connectTestDB(t)
	encKey := testSymmetricKey(t)

	nangoMock := httptest.NewServer(nangoHandler)
	t.Cleanup(nangoMock.Close)
	nangoClient := nango.NewClient(nangoMock.URL, "test-nango-secret")

	org := createTestOrg(t, db)
	user := createTestUser(t, db, fmt.Sprintf("bugsink-proxy-%s@example.com", uuid.NewString()[:8]))
	integration := createTestIntegration(t, db, "bugsink")

	agentID := seedProxyAgent(t, db, org.ID, firstTeamID(t, db, org.ID), "bugsink-proxy-agent")
	runtimeSecret := "test-runtime-secret-bugsink"
	seedProxySandbox(t, db, encKey, org.ID, agentID, runtimeSecret)

	connection := model.Connection{
		ID:                uuid.New(),
		OrgID:             org.ID,
		UserID:            user.ID,
		IntegrationID:     integration.ID,
		NangoConnectionID: "nango-bugsink-conn",
	}
	if err := db.Create(&connection).Error; err != nil {
		t.Fatalf("create connection: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", connection.ID).Delete(&model.Connection{}) })

	installTestPluginAccess(t, db, org.ID, agentID, "bugsink")

	router := chi.NewRouter()
	router.Handle("/internal/bugsink-proxy/{agentID}/*", http.HandlerFunc(handler.NewBugsinkProxyHandler(db, encKey, nangoClient).Handle))

	return &bugsinkTestHarness{
		db:            db,
		router:        router,
		encKey:        encKey,
		orgID:         org.ID,
		agentID:       agentID,
		runtimeSecret: runtimeSecret,
	}
}

func seedProxyAgent(t *testing.T, db *gorm.DB, orgID, teamID uuid.UUID, name string) uuid.UUID {
	t.Helper()
	agentID := uuid.New()
	agent := model.Agent{
		ID:     agentID,
		OrgID:  &orgID,
		TeamID: teamID,
		Name:   name,
		Status: "active",
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", agentID).Delete(&model.Agent{}) })
	return agentID
}

func seedProxySandbox(t *testing.T, db *gorm.DB, encKey *crypto.SymmetricKey, orgID, agentID uuid.UUID, runtimeSecret string) {
	t.Helper()
	encrypted, err := encKey.EncryptString(runtimeSecret)
	if err != nil {
		t.Fatalf("encrypt runtime secret: %v", err)
	}
	sandboxID := uuid.New()
	sandbox := model.Sandbox{
		ID:                     sandboxID,
		OrgID:                  &orgID,
		AgentID:                &agentID,
		Status:                 "running",
		EncryptedRuntimeSecret: encrypted,
		RuntimeURL:             "http://localhost:7080",
	}
	if err := db.Create(&sandbox).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", sandboxID).Delete(&model.Sandbox{}) })
}

func TestBugsinkProxy_TeamWithPluginForwardsThroughNango(t *testing.T) {
	called := false
	nangoHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":1,"slug":"web"}]`))
	})

	harness := newBugsinkHarness(t, nangoHandler)

	req := httptest.NewRequest(http.MethodGet,
		"/internal/bugsink-proxy/"+harness.agentID.String()+"/api/canonical/0/projects/",
		nil)
	req.Header.Set("Authorization", "Bearer "+harness.runtimeSecret)
	recorder := httptest.NewRecorder()
	harness.router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if !called {
		t.Fatal("expected nango to be called for a team that has the bugsink plugin")
	}
}

func TestBugsinkProxy_TeamWithoutPluginDenied(t *testing.T) {
	nangoHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("nango must not be called for an agent whose team lacks the bugsink plugin")
	})

	harness := newBugsinkHarness(t, nangoHandler)

	otherTeam := seedTeam(t, harness.db, harness.orgID, "bugsink-no-plugin")
	otherAgentID := seedProxyAgent(t, harness.db, harness.orgID, otherTeam.ID, "bugsink-no-plugin-agent")
	otherSecret := "test-runtime-secret-bugsink-other"
	seedProxySandbox(t, harness.db, harness.encKey, harness.orgID, otherAgentID, otherSecret)

	req := httptest.NewRequest(http.MethodGet,
		"/internal/bugsink-proxy/"+otherAgentID.String()+"/api/canonical/0/projects/",
		nil)
	req.Header.Set("Authorization", "Bearer "+otherSecret)
	recorder := httptest.NewRecorder()
	harness.router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for team without the bugsink plugin, got %d: %s", recorder.Code, recorder.Body.String())
	}
}
