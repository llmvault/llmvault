package handler_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

// templateLaunchClaims replicates the app template's launchClaims
// (global/apps/template/hivycore/auth.go) — the verifier of record.
type templateLaunchClaims struct {
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

// appLaunchBody mirrors the launch endpoint's JSON contract (iframe auth
// model): the frontend receives the token and mounts the app itself.
type appLaunchBody struct {
	Token          string `json:"token"`
	TokenExpiresIn int    `json:"token_expires_in"`
	AppURL         string `json:"app_url"`
	PreviewURL     string `json:"preview_url"`
	Status         string `json:"status"`
}

// templateLaunchParser is the template's parser configuration, verbatim.
func templateLaunchParser() *jwt.Parser {
	return jwt.NewParser(
		jwt.WithIssuer("hivy"),
		jwt.WithAudience("hivy-app"),
		jwt.WithExpirationRequired(),
		jwt.WithValidMethods([]string{"RS256"}),
	)
}

// TestAppsLaunchMintsVerifiableToken asserts the 200 JSON shape and that the
// token passes the template's exact verification contract: RS256 under the
// platform public key, iss "hivy", aud "hivy-app", required exp, matching
// app_id, a user_id, and a jti for one-time use.
func TestAppsLaunchMintsVerifiableToken(t *testing.T) {
	h := newAppsRESTHarness(t)
	appID := h.createAppViaAPI(t, "Launchable")
	h.markDeployed(t, appID)
	h.provider.endpoints[8080] = "http://127.0.0.1:34567"

	rr := h.do(t, "GET", "/v1/apps/"+appID+"/launch", nil, true)
	if rr.Code != 200 {
		t.Fatalf("launch status = %d body=%s", rr.Code, rr.Body.String())
	}
	var body appLaunchBody
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode launch body: %v", err)
	}
	if body.Token == "" {
		t.Fatal("response has no token")
	}
	if body.AppURL != "http://127.0.0.1:34567" {
		t.Fatalf("app_url = %q", body.AppURL)
	}
	if body.PreviewURL != "" {
		t.Fatalf("preview_url = %q, want empty (no preview registered)", body.PreviewURL)
	}
	if body.TokenExpiresIn != 60 {
		t.Fatalf("token_expires_in = %d, want 60", body.TokenExpiresIn)
	}
	if body.Status != model.AppStatusRunning {
		t.Fatalf("status = %q, want %q", body.Status, model.AppStatusRunning)
	}

	parser := templateLaunchParser()
	verified, err := parser.ParseWithClaims(body.Token, &templateLaunchClaims{}, func(*jwt.Token) (any, error) {
		return &h.rsaKey.PublicKey, nil
	})
	if err != nil {
		t.Fatalf("token fails the template verification: %v", err)
	}
	claims, ok := verified.Claims.(*templateLaunchClaims)
	if !ok || !verified.Valid {
		t.Fatal("invalid claims")
	}

	if claims.AppID != appID {
		t.Fatalf("app_id = %q, want %q", claims.AppID, appID)
	}
	if claims.OrgID != h.org.ID.String() {
		t.Fatalf("org_id = %q", claims.OrgID)
	}
	if claims.UserID != h.user.ID.String() {
		t.Fatalf("user_id = %q", claims.UserID)
	}
	if claims.Role != "member" {
		t.Fatalf("role = %q", claims.Role)
	}
	if claims.UserName != h.user.Name {
		t.Fatalf("user_name = %q", claims.UserName)
	}
	if claims.UserEmail != h.user.Email {
		t.Fatalf("user_email = %q", claims.UserEmail)
	}
	if claims.UserAvatar != h.user.AvatarURL {
		t.Fatalf("user_avatar = %q", claims.UserAvatar)
	}
	if claims.OrgName != h.org.Name {
		t.Fatalf("org_name = %q", claims.OrgName)
	}
	if claims.ID == "" {
		t.Fatal("missing jti (the template enforces one-time use on it)")
	}
	if claims.ExpiresAt == nil {
		t.Fatal("missing exp")
	}
	ttl := time.Until(claims.ExpiresAt.Time)
	if ttl <= 0 || ttl > 61*time.Second {
		t.Fatalf("exp ttl = %v, want ~60s", ttl)
	}

	// A second launch mints a fresh jti — tokens are one-time by design.
	second := h.do(t, "GET", "/v1/apps/"+appID+"/launch", nil, true)
	if second.Code != 200 {
		t.Fatalf("second launch status = %d body=%s", second.Code, second.Body.String())
	}
	var secondBody appLaunchBody
	if err := json.Unmarshal(second.Body.Bytes(), &secondBody); err != nil {
		t.Fatalf("decode second launch body: %v", err)
	}
	secondVerified, err := parser.ParseWithClaims(secondBody.Token, &templateLaunchClaims{}, func(*jwt.Token) (any, error) {
		return &h.rsaKey.PublicKey, nil
	})
	if err != nil {
		t.Fatalf("second token: %v", err)
	}
	if secondVerified.Claims.(*templateLaunchClaims).ID == claims.ID {
		t.Fatal("two launches minted the same jti")
	}
}

// TestAppsLaunchRequiresDeployment: with no running deployment AND no preview
// there is nowhere to mount the app, so no token is minted.
func TestAppsLaunchRequiresDeployment(t *testing.T) {
	h := newAppsRESTHarness(t)
	appID := h.createAppViaAPI(t, "Draft Only")

	rr := h.do(t, "GET", "/v1/apps/"+appID+"/launch", nil, true)
	if rr.Code != 503 {
		t.Fatalf("draft launch status = %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "not deployed") {
		t.Fatalf("draft launch body = %s", rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), `"token"`) {
		t.Fatalf("draft launch leaked a token: %s", rr.Body.String())
	}
}

// TestAppsLaunchPreviewOnly: an app that has never deployed but has a
// registered preview is launchable — 200 with a token, preview_url set, and
// app_url empty.
func TestAppsLaunchPreviewOnly(t *testing.T) {
	h := newAppsRESTHarness(t)
	appID := h.createAppViaAPI(t, "Preview Only")
	if err := h.db.Model(&model.App{}).Where("id = ?", appID).
		Update("preview_url", "http://127.0.0.1:41234").Error; err != nil {
		t.Fatalf("register preview url: %v", err)
	}

	rr := h.do(t, "GET", "/v1/apps/"+appID+"/launch", nil, true)
	if rr.Code != 200 {
		t.Fatalf("preview-only launch status = %d body=%s", rr.Code, rr.Body.String())
	}
	var body appLaunchBody
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode preview-only launch body: %v", err)
	}
	if body.Token == "" {
		t.Fatal("preview-only launch has no token")
	}
	if body.AppURL != "" {
		t.Fatalf("app_url = %q, want empty (not deployed)", body.AppURL)
	}
	if body.PreviewURL != "http://127.0.0.1:41234" {
		t.Fatalf("preview_url = %q", body.PreviewURL)
	}
	if body.Status != model.AppStatusDraft {
		t.Fatalf("status = %q, want %q", body.Status, model.AppStatusDraft)
	}
}

// TestAppsLaunchRequiresUser: API-key callers carry no user identity, so no
// launch token can be minted for them.
func TestAppsLaunchRequiresUser(t *testing.T) {
	h := newAppsRESTHarness(t)
	appID := h.createAppViaAPI(t, "No User Launch")
	h.markDeployed(t, appID)
	h.provider.endpoints[8080] = "http://127.0.0.1:34567"

	rr := h.do(t, "GET", "/v1/apps/"+appID+"/launch", nil, false)
	if rr.Code != 403 {
		t.Fatalf("userless launch status = %d body=%s", rr.Code, rr.Body.String())
	}
}

// TestAppsLaunchChannelDenied: a member who cannot use the app's
// team-restricted channel gets a 404, indistinguishable from a missing app.
func TestAppsLaunchChannelDenied(t *testing.T) {
	h := newAppsRESTHarness(t)
	appID := h.createAppViaAPI(t, "Team Locked")
	h.markDeployed(t, appID)
	h.provider.endpoints[8080] = "http://127.0.0.1:34567"

	team := model.Team{ID: uuid.New(), OrgID: h.org.ID, Name: "launch-denied-" + uuid.NewString()}
	if err := h.db.Create(&team).Error; err != nil {
		t.Fatalf("create team: %v", err)
	}
	t.Cleanup(func() { h.db.Delete(&model.Team{}, "id = ?", team.ID) })
	if err := h.db.Model(&model.Channel{}).Where("id = ?", h.channel.ID).
		Update("team_id", team.ID).Error; err != nil {
		t.Fatalf("restrict channel to team: %v", err)
	}

	rr := h.do(t, "GET", "/v1/apps/"+appID+"/launch", nil, true)
	if rr.Code != 404 {
		t.Fatalf("denied launch status = %d body=%s", rr.Code, rr.Body.String())
	}
}
