package hivycore

import (
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCallbackHappyPath(t *testing.T) {
	app, key := newTestApp(t)
	rec := callbackRequest(app, signLaunchToken(t, key, tokenOverrides{}))

	// The callback's 302 to / is the app's single internal redirect.
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/" {
		t.Fatalf("Location = %q, want /", loc)
	}
	cookie := sessionCookieFrom(t, rec)
	if cookie == nil {
		t.Fatal("no session cookie set")
	}
	// The cookie must survive the cross-origin Hivy iframe: SameSite=None,
	// Secure, Partitioned.
	if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteNoneMode || cookie.Path != "/" {
		t.Fatalf("cookie attributes wrong: %+v", cookie)
	}
	if !cookie.Partitioned || !strings.Contains(rec.Header().Get("Set-Cookie"), "Partitioned") {
		t.Fatalf("cookie is not Partitioned: %q", rec.Header().Get("Set-Cookie"))
	}

	session, err := app.codec.decrypt(cookie.Value)
	if err != nil {
		t.Fatalf("decrypting session cookie: %v", err)
	}
	if session.UserID != "6a4f0e0a-1111-4f4f-8888-aaaaaaaaaaaa" ||
		session.UserName != "Ada Lovelace" ||
		session.UserEmail != "ada@example.com" ||
		session.OrgName != "Analytical Engines" ||
		session.Role != "member" {
		t.Fatalf("session payload mismatch: %+v", session)
	}
	wantExpiry := time.Now().Add(SessionTTL).Unix()
	if session.ExpiresAt < wantExpiry-60 || session.ExpiresAt > wantExpiry+60 {
		t.Fatalf("session expiry %d not ~7 days out (%d)", session.ExpiresAt, wantExpiry)
	}
}

// assertLaunchRejected asserts a failed callback serves the 401 "not signed
// in" page (with the launch link and embedding policy) instead of
// redirecting anywhere, and sets no session cookie.
func assertLaunchRejected(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "" {
		t.Fatalf("rejected launch set Location %q; redirects are gone", loc)
	}
	if !containsAll(rec.Body.String(), "not signed in", `href="`+testLaunchURL+`"`) {
		t.Fatalf("rejected launch body missing launch link: %s", rec.Body.String())
	}
	if csp := rec.Header().Get("Content-Security-Policy"); csp != testFrameAncestors {
		t.Fatalf("rejected launch CSP = %q, want %q", csp, testFrameAncestors)
	}
	if sessionCookieFrom(t, rec) != nil {
		t.Fatal("session cookie set on rejected launch")
	}
}

func TestCallbackWrongAudience(t *testing.T) {
	app, key := newTestApp(t)
	assertLaunchRejected(t, callbackRequest(app, signLaunchToken(t, key, tokenOverrides{audience: "https://api.usehivy.test"})))
}

func TestCallbackWrongIssuer(t *testing.T) {
	app, key := newTestApp(t)
	assertLaunchRejected(t, callbackRequest(app, signLaunchToken(t, key, tokenOverrides{issuer: "not-hivy"})))
}

func TestCallbackWrongAppID(t *testing.T) {
	app, key := newTestApp(t)
	assertLaunchRejected(t, callbackRequest(app, signLaunchToken(t, key, tokenOverrides{appID: "someone-elses-app"})))
}

func TestCallbackExpiredToken(t *testing.T) {
	app, key := newTestApp(t)
	assertLaunchRejected(t, callbackRequest(app, signLaunchToken(t, key, tokenOverrides{expires: time.Now().Add(-time.Minute)})))
}

func TestCallbackWrongKey(t *testing.T) {
	app, _ := newTestApp(t)
	otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating rsa key: %v", err)
	}
	assertLaunchRejected(t, callbackRequest(app, signLaunchToken(t, otherKey, tokenOverrides{})))
}

func TestCallbackMissingToken(t *testing.T) {
	app, _ := newTestApp(t)
	assertLaunchRejected(t, callbackRequest(app, ""))
}

func TestCallbackReusedJTI(t *testing.T) {
	app, key := newTestApp(t)
	token := signLaunchToken(t, key, tokenOverrides{jti: "one-time-jti"})

	first := callbackRequest(app, token)
	if first.Code != http.StatusFound || first.Header().Get("Location") != "/" {
		t.Fatalf("first use rejected: %d %q", first.Code, first.Header().Get("Location"))
	}
	assertLaunchRejected(t, callbackRequest(app, token))
}
