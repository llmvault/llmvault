package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/usehivy/hivy/internal/middleware"
)

// API-key-authenticated requests must pass through RequireOrgAdminOrAPIKey
// (the scope ceiling is enforced in the handler). The DB is never touched on
// this path, so a nil *gorm.DB is safe here.
func TestRequireOrgAdminOrAPIKey_APIKeyPassesThrough(t *testing.T) {
	called := false
	mw := middleware.RequireOrgAdminOrAPIKey(nil)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req = middleware.WithAPIKeyClaims(req, &middleware.APIKeyClaims{
		KeyID:  "k",
		OrgID:  "o",
		Scopes: []string{"credentials"},
	})
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if !called || rr.Code != http.StatusOK {
		t.Fatalf("expected API-key request to pass through, code=%d called=%v", rr.Code, called)
	}
}

// A request that is neither API-key nor JWT-admin (no auth claims at all) must
// be rejected by the JWT-admin branch.
func TestRequireOrgAdminOrAPIKey_NoClaimsRejected(t *testing.T) {
	mw := middleware.RequireOrgAdminOrAPIKey(nil)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler must not be reached without auth")
	}))

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthenticated request, got %d", rr.Code)
	}
}
