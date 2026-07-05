package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCORS_AllowsListedOrigins(t *testing.T) {
	handler := CORS([]string{"https://app.usehivy.test"}, true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://app.usehivy.test")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://app.usehivy.test" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want %q", got, "https://app.usehivy.test")
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("Access-Control-Allow-Credentials = %q, want true", got)
	}
}

func TestCORS_BlocksUnlistedOrigins(t *testing.T) {
	handler := CORS([]string{"https://app.usehivy.test"}, true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want empty for unlisted origin", got)
	}
}

// An empty allowedOrigins slice in production must NOT emit a wildcard header (fail closed).
func TestCORS_ProductionFailsClosedWhenOriginsEmpty(t *testing.T) {
	handler := CORS([]string{}, true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://attacker.example.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want empty in production with no origins", got)
	}
}

// Non-production deployments without HIVY_CORS_ORIGINS still get the wildcard fallback behaviour.
func TestCORS_NonProductionAllowsAllWhenOriginsEmpty(t *testing.T) {
	handler := CORS([]string{}, false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want * in non-production with no origins", got)
	}
}

// Preflight OPTIONS also get no allow-origin in production with no origins.
func TestCORS_ProductionOptionsFailsClosedWhenOriginsEmpty(t *testing.T) {
	handler := CORS([]string{}, true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	req.Header.Set("Origin", "https://attacker.example.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want empty for OPTIONS in production with no origins", got)
	}
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204 for OPTIONS", rec.Code)
	}
}
