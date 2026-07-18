package handler

import (
	"crypto/rsa"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/access"
	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sheets"
)

// appActorHeader optionally carries a PLATFORM-SIGNED actor token an app
// forwards so in-app mutations are attributed to the human who acted. The value
// is a launch-style RS256 JWT the app cannot mint; a raw/forged value is never
// trusted (see appActor).
const appActorHeader = "X-Hivy-App-Actor"

// AppsInternalHandler serves the internal app API
// (/internal/apps/{appID}/v1/*) — the only Hivy surface an app backend can
// reach. Auth is the app's runtime secret as a bearer, mirroring the sandbox
// runtime-secret pattern (UploadsHandler.authAgent). Handlers are thin shells
// over sheets.Service scoped to the app's ONE bound sheet; no schema-mutation
// endpoint exists here by construction (apps plan §1.2).
type AppsInternalHandler struct {
	db        *gorm.DB
	svc       *sheets.Service
	encKey    *crypto.SymmetricKey
	presigner SheetsPresigner
	redis     redis.UniversalClient
	// actorKey is the platform's launch-token PUBLIC key. When set, appActor
	// verifies the X-Hivy-App-Actor header as a platform-signed JWT before
	// trusting the user it names. Nil disables real user attribution (fail-safe:
	// mutations are then attributed to the app's creating agent, never to an
	// unverifiable forwarded id). Wire it from the same key that mints launch
	// tokens (cmd/server) via WithActorVerifier.
	actorKey *rsa.PublicKey
}

func NewAppsInternalHandler(db *gorm.DB, svc *sheets.Service, encKey *crypto.SymmetricKey) *AppsInternalHandler {
	return &AppsInternalHandler{db: db, svc: svc, encKey: encKey}
}

// WithActorVerifier enables platform-signed X-Hivy-App-Actor attribution. pub is
// the public half of the RSA key that mints app launch tokens (apps_launch.go).
// Without it, appActor refuses to trust any forwarded actor id.
func (h *AppsInternalHandler) WithActorVerifier(pub *rsa.PublicKey) *AppsInternalHandler {
	h.actorKey = pub
	return h
}

// WithPresigner enables attachment download URLs.
func (h *AppsInternalHandler) WithPresigner(p SheetsPresigner) *AppsInternalHandler {
	h.presigner = p
	return h
}

// WithRedis enables the live sheet-event stream (GET .../v1/live). Nil leaves
// the endpoint answering 503 "not configured".
func (h *AppsInternalHandler) WithRedis(client redis.UniversalClient) *AppsInternalHandler {
	h.redis = client
	return h
}

// authApp resolves the app from the URL param and verifies the bearer matches
// the app's decrypted runtime secret (constant-time). On failure it writes the
// JSON error response and returns false — callers must return.
func (h *AppsInternalHandler) authApp(w http.ResponseWriter, r *http.Request) (*model.App, bool) {
	if h.encKey == nil || h.svc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "app endpoints not configured"})
		return nil, false
	}

	appID, err := uuid.Parse(chi.URLParam(r, "appID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid app_id"})
		return nil, false
	}

	bearer := bearerFromHeader(r.Header.Get("Authorization"))
	if bearer == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing authorization"})
		return nil, false
	}

	var app model.App
	if err := h.db.WithContext(r.Context()).
		Where("id = ? AND archived_at IS NULL", appID).
		First(&app).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "app not found"})
			return nil, false
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load app"})
		return nil, false
	}

	wantKey, err := h.encKey.DecryptString(app.EncryptedAppSecret)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "decrypt app secret", "app_id", appID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to verify credentials"})
		return nil, false
	}
	if subtle.ConstantTimeCompare([]byte(bearer), []byte(wantKey)) != 1 {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid app secret"})
		return nil, false
	}

	return &app, true
}

// appPageID parses the pageID path param and enforces that the page belongs to
// the app's bound sheet. A page of any other sheet — same org included — 404s,
// so it is indistinguishable from a missing one.
func (h *AppsInternalHandler) appPageID(w http.ResponseWriter, r *http.Request, app *model.App) (uuid.UUID, bool) {
	pageID, err := uuid.Parse(chi.URLParam(r, "pageID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "pageID must be a uuid"})
		return uuid.Nil, false
	}
	if err := h.svc.PageInSheet(r.Context(), app.OrgID, app.SheetID, pageID); err != nil {
		writeSheetsError(w, r, err)
		return uuid.Nil, false
	}
	return pageID, true
}

// appActor resolves the optional X-Hivy-App-Actor header into a sheets actor.
//
// The header must be a PLATFORM-SIGNED launch-style JWT: the app runs untrusted,
// agent-generated code, so it must not be able to name an arbitrary org member
// as the actor. We verify the token's signature with the platform launch key
// and require it to be bound to THIS app before trusting the user it carries.
//
//   - absent header: an app-only mutation, attributed to the app's creating agent.
//   - present + verifier not configured: we cannot prove who signed it, so we do
//     NOT trust it — fall back to agent attribution (a forged header can no
//     longer impersonate a user). See WithActorVerifier.
//   - present + valid signed token for this app: attributed to that user (still
//     re-checked for live org membership).
//   - present + invalid/forged token: 403.
func (h *AppsInternalHandler) appActor(w http.ResponseWriter, r *http.Request, app *model.App) (sheets.Actor, bool) {
	actor := sheets.Actor{TeamID: app.TeamID}
	raw := strings.TrimSpace(r.Header.Get(appActorHeader))
	if raw == "" {
		actor.AgentID = app.CreatedByAgentID
		return actor, true
	}
	if h.actorKey == nil {
		// No verifier wired: never trust an unverifiable forwarded id. Attribute
		// to the app's agent rather than an id the app could have forged.
		logging.FromContext(r.Context()).WarnContext(r.Context(),
			"apps_internal: ignoring app actor header — actor verification not configured", "app_id", app.ID)
		actor.AgentID = app.CreatedByAgentID
		return actor, true
	}
	userID, err := h.verifyAppActorToken(raw, app.ID)
	if err != nil {
		logging.FromContext(r.Context()).WarnContext(r.Context(), "apps_internal: rejected app actor token", "app_id", app.ID, "error", err)
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "invalid app actor token"})
		return sheets.Actor{}, false
	}
	resolved, err := access.Resolve(r.Context(), h.db, app.OrgID, userID)
	if err != nil {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: err.Error()})
		return sheets.Actor{}, false
	}
	actor.UserID = &resolved.UserID
	return actor, true
}

// verifyAppActorToken verifies raw as a platform-signed launch-style JWT (RS256,
// hivy issuer/audience) and asserts it was minted for appID. It returns the
// token's user_id only on success. Because the signing key is platform-only, the
// app cannot forge one for a different user or app.
func (h *AppsInternalHandler) verifyAppActorToken(raw string, appID uuid.UUID) (string, error) {
	var claims appLaunchClaims
	_, err := jwt.ParseWithClaims(raw, &claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method %q", t.Header["alg"])
		}
		return h.actorKey, nil
	},
		jwt.WithValidMethods([]string{"RS256"}),
		jwt.WithIssuer(appLaunchTokenIssuer),
		jwt.WithAudience(appLaunchTokenAudience),
	)
	if err != nil {
		return "", fmt.Errorf("verify actor token: %w", err)
	}
	if claims.AppID != appID.String() {
		return "", fmt.Errorf("actor token app_id %q does not match app %q", claims.AppID, appID)
	}
	if strings.TrimSpace(claims.UserID) == "" {
		return "", fmt.Errorf("actor token has no user_id")
	}
	return claims.UserID, nil
}
