package handler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

func TestMCPValidateJTIMatchAllowsSameAgentSandboxProxyTokens(t *testing.T) {
	db := connectTestDB(t)
	org := createTestOrg(t, db)
	credID := createMCPTestCredential(t, db, org.ID)
	agentID := uuid.NewString()
	sandboxID := uuid.NewString()
	urlJTI := uuid.NewString()
	bearerJTI := uuid.NewString()
	createMCPTestToken(t, db, org.ID, credID, urlJTI, agentID, sandboxID)
	createMCPTestToken(t, db, org.ID, credID, bearerJTI, agentID, sandboxID)

	status := runValidateJTIMatch(t, db, urlJTI, bearerJTI)

	if status != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", status, http.StatusNoContent)
	}
}

func TestMCPValidateJTIMatchRejectsDifferentSandboxProxyTokens(t *testing.T) {
	db := connectTestDB(t)
	org := createTestOrg(t, db)
	credID := createMCPTestCredential(t, db, org.ID)
	agentID := uuid.NewString()
	urlJTI := uuid.NewString()
	bearerJTI := uuid.NewString()
	createMCPTestToken(t, db, org.ID, credID, urlJTI, agentID, uuid.NewString())
	createMCPTestToken(t, db, org.ID, credID, bearerJTI, agentID, uuid.NewString())

	status := runValidateJTIMatch(t, db, urlJTI, bearerJTI)

	if status != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", status, http.StatusForbidden)
	}
}

func TestMCPValidateJTIMatchRejectsRevokedURLProxyToken(t *testing.T) {
	db := connectTestDB(t)
	org := createTestOrg(t, db)
	credID := createMCPTestCredential(t, db, org.ID)
	agentID := uuid.NewString()
	sandboxID := uuid.NewString()
	urlJTI := uuid.NewString()
	bearerJTI := uuid.NewString()
	createMCPTestToken(t, db, org.ID, credID, urlJTI, agentID, sandboxID)
	createMCPTestToken(t, db, org.ID, credID, bearerJTI, agentID, sandboxID)
	now := time.Now()
	if err := db.Model(&model.Token{}).Where("jti = ?", urlJTI).Update("revoked_at", now).Error; err != nil {
		t.Fatalf("revoke url token: %v", err)
	}

	status := runValidateJTIMatch(t, db, urlJTI, bearerJTI)

	if status != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", status, http.StatusForbidden)
	}
}

func runValidateJTIMatch(t *testing.T, db *gorm.DB, urlJTI, bearerJTI string) int {
	t.Helper()
	mcpHandler := handler.NewMCPHandler(db, nil, nil, nil, nil)
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	req := httptest.NewRequest(http.MethodPost, "/"+urlJTI, nil)
	routeContext := chi.NewRouteContext()
	routeContext.URLParams.Add("jti", urlJTI)
	req = req.WithContext(contextWithRoute(req, routeContext))
	req = middleware.WithClaims(req, &middleware.TokenClaims{JTI: bearerJTI})
	rr := httptest.NewRecorder()

	mcpHandler.ValidateJTIMatch(next).ServeHTTP(rr, req)
	return rr.Code
}

func contextWithRoute(req *http.Request, routeContext *chi.Context) context.Context {
	return context.WithValue(req.Context(), chi.RouteCtxKey, routeContext)
}

func createMCPTestCredential(t *testing.T, db *gorm.DB, orgID uuid.UUID) uuid.UUID {
	t.Helper()
	cred := model.Credential{
		OrgID:        &orgID,
		Label:        "mcp-test-" + uuid.NewString(),
		BaseURL:      "https://proxy.example.test",
		AuthScheme:   "bearer",
		EncryptedKey: []byte("enc"),
		WrappedDEK:   []byte("dek"),
		ProviderID:   "openrouter",
	}
	if err := db.Create(&cred).Error; err != nil {
		t.Fatalf("create credential: %v", err)
	}
	return cred.ID
}

func createMCPTestToken(t *testing.T, db *gorm.DB, orgID, credID uuid.UUID, jti, agentID, sandboxID string) {
	t.Helper()
	token := model.Token{
		OrgID:        orgID,
		CredentialID: credID,
		JTI:          jti,
		ExpiresAt:    time.Now().Add(time.Hour),
		Scopes:       model.JSON{},
		Meta: model.JSON{
			model.TokenMetaType:      model.TokenTypeAgentProxy,
			model.TokenMetaAgentID:   agentID,
			model.TokenMetaSandboxID: sandboxID,
			model.TokenMetaHarness:   model.TokenHarnessAgentSandbox,
		},
	}
	if err := db.Create(&token).Error; err != nil {
		t.Fatalf("create token: %v", err)
	}
}
