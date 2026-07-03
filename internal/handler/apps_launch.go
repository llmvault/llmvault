package handler

import (
	"errors"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/apps"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

// Launch-token contract shared with the app template
// (global/apps/template/hivycore/auth.go) — the template is the verifier, so
// these must match it exactly.
const (
	appLaunchTokenIssuer   = "hivy"
	appLaunchTokenAudience = "hivy-app"
	appLaunchTokenTTL      = 60 * time.Second
)

// appLaunchClaims is the launch JWT payload (apps plan §1.1): identity plus
// the display metadata the app renders. Field set and JSON names mirror the
// template's launchClaims.
type appLaunchClaims struct {
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

// appLaunchResponse is the launch payload for the iframe auth model: the
// frontend receives the token and mounts the app itself at
// {app_url}/auth/callback?token=... — the platform never redirects a token
// anywhere.
type appLaunchResponse struct {
	Token          string `json:"token"`
	TokenExpiresIn int    `json:"token_expires_in"` // seconds
	AppURL         string `json:"app_url"`          // deployed base URL; "" when not deployed/running
	PreviewURL     string `json:"preview_url"`      // preview base URL; "" when no preview exists
	Status         string `json:"status"`
}

// appPreviewURL returns the app's preview base URL: the preview_url the
// builder agent registered through the preview-env side channel, "" until the
// first preview registration.
func appPreviewURL(app *model.App) string {
	if app == nil {
		return ""
	}
	return app.PreviewURL
}

// Launch handles GET /v1/apps/{appID}/launch: mint the one-time launch JWT
// and return it as JSON alongside the app's URLs (iframe auth model). The
// token is RS256 under the platform auth key with a dedicated audience, so it
// is useless against the main API; the template verifies it with the public
// key only and enforces one-time jti.
// @Summary Launch an app
// @Description Mints a one-time launch token and returns it with the app's base URLs; the caller mounts the app at {app_url}/auth/callback?token=... itself. 503 when the app has neither a running deployment nor a preview.
// @Tags apps
// @Produce json
// @Param appID path string true "App ID"
// @Success 200 {object} appLaunchResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 503 {object} errorResponse
// @Security BearerAuth
// @Router /v1/apps/{appID}/launch [get]
func (h *AppsHandler) Launch(w http.ResponseWriter, r *http.Request) {
	org, app, ok := h.requireApp(w, r)
	if !ok {
		return
	}
	if h.rsaKey == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "app launch is not configured"})
		return
	}
	user, hasUser := middleware.UserFromContext(r.Context())
	if !hasUser || user == nil {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "app launch requires a user session"})
		return
	}
	role, err := h.appsOrgRole(r.Context(), org.ID, user.ID)
	if err != nil {
		logging.Capture(r.Context(), err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to resolve membership"})
		return
	}

	appURL, err := h.svc.AppURL(r.Context(), app)
	if err != nil {
		if !errors.Is(err, apps.ErrNotDeployed) {
			writeAppsError(w, r, err)
			return
		}
		appURL = "" // not deployed: still launchable when a preview exists
	}
	previewURL := appPreviewURL(app)
	if appURL == "" && previewURL == "" {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "app is not deployed or not running; publish a version first"})
		return
	}

	now := time.Now()
	claims := appLaunchClaims{
		AppID:      app.ID.String(),
		OrgID:      org.ID.String(),
		UserID:     user.ID.String(),
		Role:       role,
		UserName:   user.Name,
		UserEmail:  user.Email,
		UserAvatar: user.AvatarURL,
		OrgName:    org.Name,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    appLaunchTokenIssuer,
			Audience:  jwt.ClaimStrings{appLaunchTokenAudience},
			Subject:   user.ID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(appLaunchTokenTTL)),
			ID:        uuid.New().String(),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(h.rsaKey)
	if err != nil {
		logging.Capture(r.Context(), err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to mint launch token"})
		return
	}
	writeJSON(w, http.StatusOK, appLaunchResponse{
		Token:          token,
		TokenExpiresIn: int(appLaunchTokenTTL / time.Second),
		AppURL:         appURL,
		PreviewURL:     previewURL,
		Status:         app.Status,
	})
}
