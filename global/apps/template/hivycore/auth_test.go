package hivycore

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	testAppID     = "0f7d8f0e-3333-4f4f-aaaa-cccccccccccc"
	testLaunchURL = "https://app.usehivy.test/w/apps/0f7d8f0e/launch"
	// testFrameAncestors is the CSP derived from testLaunchURL's origin.
	testFrameAncestors = "frame-ancestors 'self' https://app.usehivy.test"
)

// newTestApp builds an App around a fresh RSA keypair and a temp public dir,
// returning the signing key for minting launch tokens.
func newTestApp(t *testing.T) (*App, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating rsa key: %v", err)
	}
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatalf("marshaling public key: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})

	pub, err := parseRSAPublicKey(string(pemBytes))
	if err != nil {
		t.Fatalf("parseRSAPublicKey: %v", err)
	}

	publicDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(publicDir, "index.html"), []byte("<!doctype html><title>t</title>"), 0o644); err != nil {
		t.Fatalf("writing index.html: %v", err)
	}

	app, err := newApp(&Config{
		AppID:         testAppID,
		AppSecret:     "hvapp_testsecret",
		APIBaseURL:    "http://sheets.invalid",
		AuthPublicKey: pub,
		LaunchURL:     testLaunchURL,
		SessionSecret: "test-session-secret",
		SheetID:       "9d1c2b3a-4444-4f4f-bbbb-dddddddddddd",
		Port:          "8080",
		PublicDir:     publicDir,
	})
	if err != nil {
		t.Fatalf("newApp: %v", err)
	}
	return app, key
}

type tokenOverrides struct {
	appID    string
	audience string
	issuer   string
	expires  time.Time
	jti      string
}

func signLaunchToken(t *testing.T, key *rsa.PrivateKey, o tokenOverrides) string {
	t.Helper()
	if o.appID == "" {
		o.appID = testAppID
	}
	if o.audience == "" {
		o.audience = launchTokenAudience
	}
	if o.issuer == "" {
		o.issuer = launchTokenIssuer
	}
	if o.expires.IsZero() {
		o.expires = time.Now().Add(60 * time.Second)
	}
	if o.jti == "" {
		o.jti = "jti-" + t.Name()
	}
	claims := launchClaims{
		AppID:      o.appID,
		OrgID:      "7b5f1f1b-2222-4f4f-9999-bbbbbbbbbbbb",
		UserID:     "6a4f0e0a-1111-4f4f-8888-aaaaaaaaaaaa",
		Role:       "member",
		UserName:   "Ada Lovelace",
		UserEmail:  "ada@example.com",
		UserAvatar: "https://cdn.example.com/ada.png",
		OrgName:    "Analytical Engines",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    o.issuer,
			Audience:  jwt.ClaimStrings{o.audience},
			Subject:   "6a4f0e0a-1111-4f4f-8888-aaaaaaaaaaaa",
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(o.expires),
			ID:        o.jti,
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(key)
	if err != nil {
		t.Fatalf("signing launch token: %v", err)
	}
	return signed
}

func callbackRequest(app *App, token string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/auth/callback?token="+token, nil)
	app.Handler().ServeHTTP(rec, req)
	return rec
}

func sessionCookieFrom(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == SessionCookieName {
			return c
		}
	}
	return nil
}

func TestAPIMiddleware(t *testing.T) {
	app, key := newTestApp(t)
	app.HandleAPI("GET /whoami", func(w http.ResponseWriter, r *http.Request) {
		user, ok := UserFrom(r.Context())
		if !ok {
			t.Error("UserFrom returned no session inside authed handler")
			WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "no session"})
			return
		}
		WriteJSON(w, http.StatusOK, map[string]string{"user_id": user.UserID})
	})
	handler := app.Handler()

	// No session → 401 JSON with the launch URL.
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/whoami", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated /api status = %d, want 401", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Fatalf("401 content type = %q", ct)
	}
	body := rec.Body.String()
	if !containsAll(body, `"unauthorized"`, testLaunchURL) {
		t.Fatalf("401 body missing launch_url: %s", body)
	}

	// Establish a session via the callback, then replay the cookie.
	cookie := sessionCookieFrom(t, callbackRequest(app, signLaunchToken(t, key, tokenOverrides{})))
	if cookie == nil {
		t.Fatal("no session cookie from callback")
	}
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/whoami", nil)
	req.AddCookie(cookie)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("authed /api status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	if !containsAll(rec.Body.String(), "6a4f0e0a-1111-4f4f-8888-aaaaaaaaaaaa") {
		t.Fatalf("authed /api body = %s", rec.Body.String())
	}

	// Tampered cookie → 401.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/whoami", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "AAAA" + cookie.Value[4:]})
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("tampered cookie /api status = %d, want 401", rec.Code)
	}
}

func TestPageGateAndHealthz(t *testing.T) {
	app, key := newTestApp(t)
	handler := app.Handler()

	// Unauthenticated HTML navigation → the 401 "not signed in" page with a
	// plain link to the launch URL. Never a redirect.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("page nav status = %d, want 401", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "" {
		t.Fatalf("page nav set Location %q; redirects are gone", loc)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("401 page content type = %q", ct)
	}
	if !containsAll(rec.Body.String(), "not signed in", `href="`+testLaunchURL+`"`) {
		t.Fatalf("401 page body missing launch link: %s", rec.Body.String())
	}
	if csp := rec.Header().Get("Content-Security-Policy"); csp != testFrameAncestors {
		t.Fatalf("401 page CSP = %q, want %q", csp, testFrameAncestors)
	}

	// Authenticated navigation serves the SPA with the embedding policy.
	cookie := sessionCookieFrom(t, callbackRequest(app, signLaunchToken(t, key, tokenOverrides{})))
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/some/client/route", nil)
	req.Header.Set("Accept", "text/html")
	req.AddCookie(cookie)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !containsAll(rec.Body.String(), "<!doctype html>") {
		t.Fatalf("authed SPA nav: %d %s", rec.Code, rec.Body.String())
	}
	if csp := rec.Header().Get("Content-Security-Policy"); csp != testFrameAncestors {
		t.Fatalf("SPA CSP = %q, want %q", csp, testFrameAncestors)
	}

	// Non-HTML asset requests are neither redirected nor gated.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/assets/missing.js", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code == http.StatusFound || rec.Code == http.StatusUnauthorized {
		t.Fatalf("asset request blocked: %d", rec.Code)
	}

	// /healthz needs no auth and reports the template version.
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK || !containsAll(rec.Body.String(), `"ok"`, TemplateVersion) {
		t.Fatalf("healthz: %d %s", rec.Code, rec.Body.String())
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}
