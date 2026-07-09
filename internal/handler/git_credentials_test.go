package handler_test

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/nango"
	"github.com/usehivy/hivy/internal/testdb"
)

const gitCredsTestDBURL = testdb.DefaultDatabaseURL

func testSymmetricKey(t *testing.T) *crypto.SymmetricKey {
	t.Helper()
	key := make([]byte, 32)
	for idx := range key {
		key[idx] = byte(idx + 42)
	}
	encKey, err := crypto.NewSymmetricKey(base64.StdEncoding.EncodeToString(key))
	if err != nil {
		t.Fatal(err)
	}
	return encKey
}

type gitCredsHarness struct {
	db            *gorm.DB
	router        *chi.Mux
	encKey        *crypto.SymmetricKey
	orgID         uuid.UUID
	agentID       uuid.UUID
	sandboxID     uuid.UUID
	runtimeSecret string
	nangoMock     *httptest.Server
}

func newGitCredsHarness(t *testing.T, nangoHandler http.Handler) *gitCredsHarness {
	t.Helper()

	dsn := testdb.DatabaseURL("DATABASE_URL", "HIVY_DATABASE_URL", "TEST_DATABASE_URL")
	database, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("cannot connect to test database: %v", err)
	}
	testdb.ApplyMigrations(t, database)

	encKey := testSymmetricKey(t)

	nangoMock := httptest.NewServer(nangoHandler)
	t.Cleanup(nangoMock.Close)

	nangoClient := nango.NewClient(nangoMock.URL, "test-nango-secret")

	gitCredsHandler := handler.NewGitCredentialsHandler(database, encKey, nangoClient)

	orgID := uuid.New()
	org := model.Org{
		ID:        orgID,
		Name:      fmt.Sprintf("gitcreds-test-%s", uuid.New().String()[:8]),
		RateLimit: 1000,
		Active:    true,
	}
	if err := database.Create(&org).Error; err != nil {
		t.Fatalf("create test org: %v", err)
	}

	agentID := uuid.New()
	agent := model.Agent{
		ID:     agentID,
		OrgID:  &orgID,
		TeamID: firstTeamID(t, database, orgID),
		Name:   "test-agent",
		Status: "active",
	}
	if err := database.Create(&agent).Error; err != nil {
		t.Fatalf("create test agent: %v", err)
	}

	runtimeSecret := "test-runtime-api-key-for-git-creds"
	encryptedKey, err := encKey.EncryptString(runtimeSecret)
	if err != nil {
		t.Fatalf("encrypt runtime secret: %v", err)
	}

	sandboxID := uuid.New()
	sandbox := model.Sandbox{
		ID:                     sandboxID,
		OrgID:                  &orgID,
		AgentID:                &agentID,
		EncryptedRuntimeSecret: encryptedKey,
		Status:                 "running",
		ExternalID:             "mock-external-id",
		RuntimeURL:             "http://localhost:25434",
	}
	if err := database.Create(&sandbox).Error; err != nil {
		t.Fatalf("create test sandbox: %v", err)
	}

	userID := uuid.New()
	user := model.User{
		ID:    userID,
		Email: fmt.Sprintf("gitcreds-test-%s@example.com", uuid.New().String()[:8]),
		Name:  "Test User",
	}
	if err := database.Create(&user).Error; err != nil {
		t.Fatalf("create test user: %v", err)
	}

	integration := createTestIntegration(t, database, "github-app")

	connectionID := uuid.New()
	connection := model.Connection{
		ID:                connectionID,
		OrgID:             orgID,
		UserID:            userID,
		IntegrationID:     integration.ID,
		NangoConnectionID: "nango-conn-123",
	}
	if err := database.Create(&connection).Error; err != nil {
		t.Fatalf("create test in_connection: %v", err)
	}

	installTestPluginAccess(t, database, orgID, agentID, "github-app")

	t.Cleanup(func() {
		database.Where("org_id = ?", orgID).Delete(&model.Connection{})
		database.Where("id = ?", sandboxID).Delete(&model.Sandbox{})
		database.Where("org_id = ?", orgID).Delete(&model.Agent{})
		database.Where("id = ?", userID).Delete(&model.User{})
		database.Where("id = ?", orgID).Delete(&model.Org{})
	})

	router := chi.NewRouter()
	router.Post("/internal/git-credentials/{agentID}", gitCredsHandler.Handle)

	return &gitCredsHarness{
		db:            database,
		router:        router,
		encKey:        encKey,
		orgID:         orgID,
		agentID:       agentID,
		sandboxID:     sandboxID,
		runtimeSecret: runtimeSecret,
		nangoMock:     nangoMock,
	}
}

func performGitCredsRequest(t *testing.T, harness *gitCredsHarness) string {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost,
		"/internal/git-credentials/"+harness.agentID.String(), nil)
	req.Header.Set("Authorization", "Bearer "+harness.runtimeSecret)
	recorder := httptest.NewRecorder()
	harness.router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if ct := recorder.Header().Get("Content-Type"); ct != "text/plain" {
		t.Fatalf("expected Content-Type text/plain, got %q", ct)
	}

	return recorder.Body.String()
}
