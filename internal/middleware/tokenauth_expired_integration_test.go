package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/token"
)

type recordingExpiredProxyTokenHandler struct {
	called bool
	token  model.Token
}

func (h *recordingExpiredProxyTokenHandler) HandleExpiredProxyToken(_ context.Context, tok model.Token) error {
	h.called = true
	h.token = tok
	return nil
}

func TestIntegration_TokenAuth_ExpiredEmployeeProxyTokenCallsHandler(t *testing.T) {
	db := connectTestDB(t)

	orgID := uuid.New()
	credID := uuid.New()

	org := model.Org{
		ID:        orgID,
		Name:      "integration-expired-employee-token-org",
		RateLimit: 1000,
		Active:    true,
	}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("failed to create org: %v", err)
	}
	cred := model.Credential{
		ID:           credID,
		OrgID:        &orgID,
		Label:        "test-cred",
		BaseURL:      "https://api.example.com",
		AuthScheme:   "bearer",
		EncryptedKey: []byte("fake-encrypted"),
		WrappedDEK:   []byte("fake-wrapped"),
	}
	if err := db.Create(&cred).Error; err != nil {
		t.Fatalf("failed to create credential: %v", err)
	}
	t.Cleanup(func() { cleanupOrg(t, db, orgID) })

	signingKey := []byte(testSigningKey)
	tokenStr, jti, err := token.Mint(signingKey, orgID.String(), credID.String(), -time.Hour)
	if err != nil {
		t.Fatalf("failed to mint token: %v", err)
	}
	tokenRecord := model.Token{
		ID:           uuid.New(),
		OrgID:        orgID,
		CredentialID: credID,
		JTI:          jti,
		ExpiresAt:    time.Now().Add(-time.Hour),
		Meta: model.JSON{
			model.TokenMetaType:        model.TokenTypeEmployeeProxy,
			model.TokenMetaHarness:     model.TokenHarnessEmployeeSandbox,
			model.TokenMetaRuntimeMode: model.TokenRuntimeModeEmployee,
		},
	}
	if err := db.Create(&tokenRecord).Error; err != nil {
		t.Fatalf("failed to create token record: %v", err)
	}
	expiredHandler := &recordingExpiredProxyTokenHandler{}
	handler := middleware.TokenAuth(signingKey, db, middleware.WithExpiredProxyTokenHandler(expiredHandler))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called for expired token")
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1/proxy/chat", nil)
	req.Header.Set("Authorization", "Bearer ptok_"+tokenStr)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
	if !expiredHandler.called {
		t.Fatal("expired proxy token handler was not called")
	}
	if expiredHandler.token.JTI != jti {
		t.Fatalf("handler token jti = %q, want %q", expiredHandler.token.JTI, jti)
	}
}
