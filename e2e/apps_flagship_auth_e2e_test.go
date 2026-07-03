package e2e

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/usehivy/hivy/internal/auth"
	"github.com/usehivy/hivy/internal/model"
)

// appsFlagshipLaunchClaims replicates the app template's launchClaims
// (global/apps/template/hivycore/auth.go) — the verifier of record.
type appsFlagshipLaunchClaims struct {
	AppID      string `json:"app_id"`
	OrgID      string `json:"org_id"`
	UserID     string `json:"user_id"`
	Role       string `json:"role"`
	UserName   string `json:"user_name"`
	UserEmail  string `json:"user_email"`
	UserAvatar string `json:"user_avatar"`
	OrgName    string `json:"org_name"`
	jwt.RegisteredClaims
}

// appsFlagshipAuthPublicKey derives the platform's auth public key from the
// stack's HIVY_AUTH_RSA_PRIVATE_KEY (loaded from .env) — the same key the api
// container signs launch tokens with and the app template verifies against.
func appsFlagshipAuthPublicKey(t *testing.T) *rsa.PublicKey {
	t.Helper()
	raw := strings.TrimSpace(os.Getenv("HIVY_AUTH_RSA_PRIVATE_KEY"))
	if raw == "" {
		t.Fatalf("HIVY_AUTH_RSA_PRIVATE_KEY is required to verify launch tokens")
	}
	key, err := auth.LoadRSAPrivateKey(raw)
	if err != nil {
		t.Fatalf("load auth RSA key: %v", err)
	}
	return &key.PublicKey
}

// assertAppsFlagshipLaunchHandoff is pillar (d), iframe auth model: GET
// /v1/apps/{id}/launch answers 200 JSON {token, token_expires_in, app_url,
// preview_url, status}; the one-time RS256 token verifies claim-by-claim
// under the platform public key; mounting the app at
// {app_url}/auth/callback?token=… sets the SameSite=None; Secure;
// Partitioned session cookie and 302s to / (the app's ONLY redirect);
// /api/me identifies the owner; and REPLAYING the same token gets the 401
// page with no cookie. Returns the session cookie value.
func assertAppsFlagshipLaunchHandoff(t *testing.T, ctx context.Context, apiBase, appBase, token, orgID string, app *model.App, owner appsFlagshipOwner) string {
	t.Helper()
	authHeaders := map[string]string{"Authorization": "Bearer " + token, "X-Org-ID": orgID}
	launch := appsFlagshipDo(t, ctx, http.MethodGet, apiBase+"/v1/apps/"+app.ID.String()+"/launch", authHeaders, nil)
	if launch.Status != http.StatusOK {
		t.Fatalf("GET /v1/apps/%s/launch status=%d want 200; body=%s", app.ID, launch.Status, launch.Body)
	}
	var body struct {
		Token          string `json:"token"`
		TokenExpiresIn int    `json:"token_expires_in"`
		AppURL         string `json:"app_url"`
		PreviewURL     string `json:"preview_url"`
		Status         string `json:"status"`
	}
	if err := json.Unmarshal(launch.Body, &body); err != nil {
		t.Fatalf("decode launch response: %v body=%s", err, launch.Body)
	}
	if body.Token == "" {
		t.Fatalf("launch response has no token: %s", launch.Body)
	}
	if body.TokenExpiresIn != 60 {
		t.Fatalf("launch token_expires_in=%d want 60", body.TokenExpiresIn)
	}
	if body.Status != model.AppStatusRunning {
		t.Fatalf("launch status=%q want running", body.Status)
	}
	if body.PreviewURL != app.PreviewURL {
		t.Fatalf("launch preview_url=%q want the registered %q", body.PreviewURL, app.PreviewURL)
	}
	// The app_url must resolve to the same published port the docker port
	// binding gave us — i.e. the exact app we probed in pillar (c).
	if got := appsFlagshipRewriteHost(t, body.AppURL); got != appBase {
		t.Fatalf("launch app_url=%q (host view %q) want %q", body.AppURL, got, appBase)
	}
	t.Logf("launch answered 200 JSON: status=%s app_url=%s preview_url=%s", body.Status, body.AppURL, body.PreviewURL)

	// Claim-by-claim verification under the template's exact parser contract.
	parser := jwt.NewParser(
		jwt.WithIssuer("hivy"),
		jwt.WithAudience("hivy-app"),
		jwt.WithExpirationRequired(),
		jwt.WithValidMethods([]string{"RS256"}),
	)
	pub := appsFlagshipAuthPublicKey(t)
	parsed, err := parser.ParseWithClaims(body.Token, &appsFlagshipLaunchClaims{}, func(*jwt.Token) (any, error) {
		return pub, nil
	})
	if err != nil {
		t.Fatalf("launch token fails the template verification contract: %v", err)
	}
	claims, ok := parsed.Claims.(*appsFlagshipLaunchClaims)
	if !ok || !parsed.Valid {
		t.Fatalf("launch token claims invalid")
	}
	if claims.AppID != app.ID.String() {
		t.Fatalf("token app_id=%q want %q", claims.AppID, app.ID)
	}
	if claims.OrgID != orgID {
		t.Fatalf("token org_id=%q want %q", claims.OrgID, orgID)
	}
	if claims.UserID != owner.UserID || claims.Subject != owner.UserID {
		t.Fatalf("token user identity: user_id=%q sub=%q want %q", claims.UserID, claims.Subject, owner.UserID)
	}
	if claims.UserEmail != owner.Email || claims.UserName != owner.Name {
		t.Fatalf("token user metadata: email=%q name=%q want %q / %q", claims.UserEmail, claims.UserName, owner.Email, owner.Name)
	}
	if claims.Role == "" || claims.OrgName == "" {
		t.Fatalf("token missing role/org_name: role=%q org_name=%q", claims.Role, claims.OrgName)
	}
	if claims.ID == "" {
		t.Fatalf("token missing jti (one-time use depends on it)")
	}
	if claims.ExpiresAt == nil || claims.IssuedAt == nil {
		t.Fatalf("token missing exp/iat")
	}
	if ttl := claims.ExpiresAt.Sub(claims.IssuedAt.Time); ttl != 60*time.Second {
		t.Fatalf("token ttl=%v want exactly 60s", ttl)
	}
	t.Logf("launch token verified claim-by-claim (jti=%s role=%s)", claims.ID, claims.Role)

	// Mount the app at the callback, the way the frontend iframe does.
	callbackURL := appBase + "/auth/callback?token=" + url.QueryEscape(body.Token)
	callback := appsFlagshipDo(t, ctx, http.MethodGet, callbackURL, nil, nil)
	if callback.Status != http.StatusFound || callback.Header.Get("Location") != "/" {
		t.Fatalf("auth callback status=%d location=%q want 302 → /; body=%s", callback.Status, callback.Header.Get("Location"), callback.Body)
	}
	rawCookie := appsFlagshipSessionCookieRaw(callback.Header)
	if rawCookie == "" {
		t.Fatalf("auth callback did not set the hivy_app_session cookie: %v", callback.Header["Set-Cookie"])
	}
	for _, attr := range []string{"SameSite=None", "Secure", "Partitioned", "HttpOnly"} {
		if !strings.Contains(rawCookie, attr) {
			t.Fatalf("session cookie missing %s attribute (iframe auth model): %q", attr, rawCookie)
		}
	}
	cookie := appsFlagshipCookieValue(rawCookie)
	t.Log("auth callback set the SameSite=None; Secure; Partitioned session cookie and redirected to /")

	me := appsFlagshipDo(t, ctx, http.MethodGet, appBase+"/api/me", map[string]string{"Cookie": "hivy_app_session=" + cookie}, nil)
	if me.Status != http.StatusOK {
		t.Fatalf("GET /api/me status=%d body=%s", me.Status, me.Body)
	}
	var session struct {
		UserID    string `json:"user_id"`
		UserName  string `json:"user_name"`
		UserEmail string `json:"user_email"`
		OrgID     string `json:"org_id"`
		Role      string `json:"role"`
	}
	if err := json.Unmarshal(me.Body, &session); err != nil {
		t.Fatalf("decode /api/me: %v body=%s", err, me.Body)
	}
	if session.UserID != owner.UserID || session.UserEmail != owner.Email || session.UserName != owner.Name || session.OrgID != orgID || session.Role == "" {
		t.Fatalf("/api/me identity mismatch: got %+v want user=%s email=%s name=%s org=%s", session, owner.UserID, owner.Email, owner.Name, orgID)
	}
	t.Logf("/api/me identified the owner: %s (%s) role=%s", session.UserName, session.UserEmail, session.Role)

	// Replaying the SAME one-time token must get the 401 page — no redirect,
	// no session cookie.
	replay := appsFlagshipDo(t, ctx, http.MethodGet, callbackURL, nil, nil)
	if replay.Status != http.StatusUnauthorized {
		t.Fatalf("replayed callback status=%d want 401; body=%s", replay.Status, replay.Body)
	}
	if loc := replay.Header.Get("Location"); loc != "" {
		t.Fatalf("replayed callback redirected (Location=%q) — the app must never redirect a failed launch", loc)
	}
	if !strings.Contains(string(replay.Body), "not signed in") {
		t.Fatalf("replayed callback did not serve the not-signed-in page: %s", replay.Body)
	}
	if appsFlagshipSessionCookieRaw(replay.Header) != "" {
		t.Fatalf("replayed one-time token set a session cookie: %v", replay.Header["Set-Cookie"])
	}
	t.Log("one-time launch token replay rejected (401 page, no cookie)")
	return cookie
}

// appsFlagshipSessionCookieRaw returns the raw Set-Cookie line for
// hivy_app_session with a non-empty value, or "".
func appsFlagshipSessionCookieRaw(header http.Header) string {
	for _, raw := range header.Values("Set-Cookie") {
		if !strings.HasPrefix(raw, "hivy_app_session=") {
			continue
		}
		if appsFlagshipCookieValue(raw) != "" {
			return raw
		}
	}
	return ""
}

// appsFlagshipCookieValue extracts the cookie value from a raw Set-Cookie line.
func appsFlagshipCookieValue(raw string) string {
	value := strings.TrimPrefix(raw, "hivy_app_session=")
	if i := strings.Index(value, ";"); i >= 0 {
		value = value[:i]
	}
	return value
}
