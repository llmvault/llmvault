// DO NOT EDIT — hivycore is the Hivy platform core. See doc.go.

package hivycore

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Launch tokens are minted by the Hivy platform with a dedicated audience so
// they are useless against the main API, and verified here with the public
// key only — the app never holds a signing secret.
const (
	launchTokenIssuer   = "hivy"
	launchTokenAudience = "hivy-app"

	// jtiRetention must exceed the longest possible launch-token lifetime
	// (60s) so a token can never outlive its replay record.
	jtiRetention = 5 * time.Minute
	jtiCapacity  = 10000
)

// launchClaims is the launch JWT payload: identity plus the display metadata
// the app renders. It is the source of every session field.
type launchClaims struct {
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

// handleAuthCallback is GET /auth/callback?token=…: verify the one-time
// launch JWT, mint the encrypted session cookie, and land the user on the
// SPA (the 302 to / below is the app's single internal redirect). The Hivy
// frontend mounts the app iframe directly at this URL after fetching a
// launch token; every failure serves the 401 "not signed in" page — the app
// never redirects out to Hivy.
func (a *App) handleAuthCallback(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		a.failLaunch(w, errors.New("missing token parameter"))
		return
	}

	claims, err := a.verifyLaunchToken(token)
	if err != nil {
		a.failLaunch(w, err)
		return
	}

	session := Session{
		UserID:     claims.UserID,
		UserName:   claims.UserName,
		UserEmail:  claims.UserEmail,
		UserAvatar: claims.UserAvatar,
		OrgID:      claims.OrgID,
		OrgName:    claims.OrgName,
		Role:       claims.Role,
		ExpiresAt:  time.Now().Add(SessionTTL).Unix(),
	}
	value, err := a.codec.encrypt(session)
	if err != nil {
		a.failLaunch(w, fmt.Errorf("sealing session: %w", err))
		return
	}

	http.SetCookie(w, sessionCookie(value))
	a.log.Info("session established", "user_id", claims.UserID, "org_id", claims.OrgID)
	http.Redirect(w, r, "/", http.StatusFound)
}

// verifyLaunchToken enforces the full launch-token contract: RS256 under the
// platform public key, issuer "hivy", audience "hivy-app", a required exp,
// an app_id matching this app, a user_id, and a one-time jti.
func (a *App) verifyLaunchToken(token string) (*launchClaims, error) {
	parser := jwt.NewParser(
		jwt.WithIssuer(launchTokenIssuer),
		jwt.WithAudience(launchTokenAudience),
		jwt.WithExpirationRequired(),
		jwt.WithValidMethods([]string{"RS256"}),
	)
	parsed, err := parser.ParseWithClaims(token, &launchClaims{}, func(*jwt.Token) (any, error) {
		return a.cfg.AuthPublicKey, nil
	})
	if err != nil {
		return nil, fmt.Errorf("validating launch token: %w", err)
	}
	claims, ok := parsed.Claims.(*launchClaims)
	if !ok || !parsed.Valid {
		return nil, errors.New("invalid launch token claims")
	}
	if claims.AppID != a.cfg.AppID {
		return nil, errors.New("launch token app_id does not match this app")
	}
	if claims.UserID == "" {
		return nil, errors.New("launch token missing user_id")
	}
	if claims.ID == "" {
		return nil, errors.New("launch token missing jti")
	}
	if !a.jti.firstUse(claims.ID) {
		return nil, errors.New("launch token already used")
	}
	return claims, nil
}

func (a *App) failLaunch(w http.ResponseWriter, err error) {
	a.log.Warn("launch callback rejected", "error", err.Error())
	a.writeNotAuthenticated(w)
}
